package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"github.com/volte6/gomud/internal/configs"
	"github.com/volte6/gomud/internal/mudlog"
	"github.com/volte6/gomud/internal/util"
)

var (
	httpServer  *http.Server
	httpsServer *http.Server

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	httpRoot = ``

	// Used to interface with plugins and request web stuff
	webPlugins WebPlugin = nil
)

type WebNav struct {
	Name   string
	Target string
}

type WebPlugin interface {
	NavLinks() map[string]string                                                    // Name=>Path pairs
	WebRequest(r *http.Request) (html string, templateData map[string]any, ok bool) // Get the first handler of a given request
}

func SetWebPlugin(wp WebPlugin) {
	webPlugins = wp
}

// serveTemplate searches for the requested file in the HTTP_ROOT,
// parses it as a template, and serves it.
func serveTemplate(w http.ResponseWriter, r *http.Request) {

	if httpRoot == "" {
		httpRoot = filepath.Clean(configs.GetFilePathsConfig().PublicHtml.String())
	}

	// Clean the path to prevent directory traversal.
	reqPath := filepath.Clean(r.URL.Path) // Example: / or /info/faq

	// Build the full file path.
	fullPath := filepath.Join(httpRoot, reqPath)

	// If the path is a directory, look for an index.html.
	info, err := os.Stat(fullPath)
	if err != nil {
		if filepath.Ext(fullPath) != ".html" {
			fullPath += ".html"
		}
	} else if info.IsDir() {
		fullPath = filepath.Join(fullPath, "index.html")
	}

	fileExt := filepath.Ext(fullPath)
	fileBase := filepath.Base(fullPath)

	// All template files to load from the filesystem
	templateFiles := []string{}

	var pageFound bool = true

	var pluginHtml string = ``
	var pluginTplData map[string]any = nil
	var ok bool = false
	var fSize int64 = 0
	var source string = `PublicHtml folder`

	// Check if the file exists, else 404
	fInfo, err := os.Stat(fullPath)
	if err != nil {
		pageFound = false
	}

	// Allow plugin to override request
	if webPlugins != nil {
		pluginHtml, pluginTplData, ok = webPlugins.WebRequest(r)
		fSize = int64(len([]byte(pluginHtml)))
		if ok {
			source = `module`
			pageFound = true
		}
	}

	if !pageFound || len(fileBase) > 0 && fileBase[0] == '_' {
		mudlog.Info("Web", "ip", r.RemoteAddr, "ref", r.Header.Get("Referer"), "file path", fullPath, "file extension", fileExt, "error", "Not found")

		fullPath = filepath.Join(httpRoot, `404.html`)
		fInfo, err = os.Stat(fullPath)

		if err != nil {
			http.NotFound(w, r)
			return
		}

		fSize = fInfo.Size()

		w.WriteHeader(http.StatusNotFound)
	}

	// Log the request
	mudlog.Info("Web", "ip", r.RemoteAddr, "ref", r.Header.Get("Referer"), "file path", fullPath, "file extension", fileExt, "file source", source, "size", fmt.Sprintf(`%.2fk`, float64(fSize)/1024))

	// For non-HTML files, serve them statically.
	if fileExt != ".html" {
		http.ServeFile(w, r, fullPath)
		return
	}

	templateData := map[string]any{
		"REQUEST": r,
		"CONFIG":  configs.GetConfig(),
		"STATS":   GetStats(),
		"NAV": []WebNav{
			{`Home`, `/`},
			{`Who's Online`, `/online`},
			{`Web Client`, `/webclient`},
			{`See Configuration`, `/viewconfig`},
		},
	}

	// Copy any plugin navigation
	if webPlugins != nil {

		currentNav := templateData[`NAV`].([]WebNav)

		for name, path := range webPlugins.NavLinks() {

			found := false
			for i := len(currentNav) - 1; i >= 0; i-- {

				if currentNav[i].Name == name {
					found = true
					if path == `` {
						currentNav = append(currentNav[:i], currentNav[i+1:]...)
					} else {
						currentNav[i].Target = path
					}
					break
				}

			}

			if !found {
				currentNav = append(currentNav, WebNav{name, path})
			}
		}

		templateData[`NAV`] = currentNav
	}

	// Copy over any plugin data loaded.
	for name, value := range pluginTplData {
		// Don't allow overwriting defaults
		if _, ok := templateData[name]; !ok {
			templateData[name] = value
		}
	}

	// Parse special files intended to be used as template includes
	globFiles, err := filepath.Glob(filepath.Join(httpRoot, "_*.html"))
	if err == nil {
		templateFiles = append(templateFiles, globFiles...)
	}

	// Parse special files intended to be used as template includes (from the request folder)
	requestDir := filepath.Dir(fullPath)
	if httpRoot != requestDir {
		globFiles, err = filepath.Glob(filepath.Join(requestDir, "_*.html"))
		if err == nil {
			templateFiles = append(templateFiles, globFiles...)
		}
	}

	// Add the final (actual) file

	// Parse
	tmpl := template.New(filepath.Base(fullPath)).Funcs(funcMap)

	if pluginHtml == `` {
		templateFiles = append(templateFiles, fullPath)

	}

	tmpl, err = tmpl.ParseFiles(templateFiles...)
	if err != nil {
		mudlog.Error("HTML ERROR", "action", "ParseFiles", "error", err)
		http.Error(w, "Error parsing template files", http.StatusInternalServerError)
	}

	if pluginHtml != `` {
		tmpl, err = tmpl.Parse(pluginHtml)
		if err != nil {
			mudlog.Error("HTML ERROR", "action", "Parse", "error", err)
			http.Error(w, "Error parsing plugin html", http.StatusInternalServerError)
		}
	}

	// Execute the template and write it to the response.
	if err := tmpl.Execute(w, templateData); err != nil {
		mudlog.Error("HTML ERROR", "action", "Execute", "error", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
	}
}

func Listen(webPort int, webHttpsPort int, wg *sync.WaitGroup, webSocketHandler func(*websocket.Conn)) {

	// Routing
	// Basic homepage
	http.HandleFunc("/", serveTemplate)

	// websocket upgrade
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("WebSocket upgrade failed:", err)
			return
		}
		defer conn.Close()

		webSocketHandler(conn)
	})

	http.Handle("GET /admin/static/", RunWithMUDLocked(
		doBasicAuth(
			handlerToHandlerFunc(
				http.StripPrefix("/admin/static/", http.FileServer(http.Dir(configs.GetFilePathsConfig().AdminHtml.String()+"/static"))),
			),
		),
	))

	// Admin tools
	http.HandleFunc("GET /admin/", RunWithMUDLocked(
		doBasicAuth(adminIndex),
	))

	// Item Admin
	http.HandleFunc("GET /admin/items/", RunWithMUDLocked(
		doBasicAuth(itemsIndex),
	))
	http.HandleFunc("GET /admin/items/itemdata/", RunWithMUDLocked(
		doBasicAuth(itemData),
	))

	// Race Admin
	http.HandleFunc("GET /admin/races/", RunWithMUDLocked(
		doBasicAuth(racesIndex)),
	)
	http.HandleFunc("GET /admin/races/racedata/", RunWithMUDLocked(
		doBasicAuth(raceData)),
	)

	// Mob Admin
	http.HandleFunc("GET /admin/mobs/", RunWithMUDLocked(
		doBasicAuth(mobsIndex),
	))
	http.HandleFunc("GET /admin/mobs/mobdata/", RunWithMUDLocked(
		doBasicAuth(mobData),
	))

	// Mutator Admin
	http.HandleFunc("GET /admin/mutators/", RunWithMUDLocked(
		doBasicAuth(mutatorsIndex),
	))
	http.HandleFunc("GET /admin/mutators/mutatordata/", RunWithMUDLocked(
		doBasicAuth(mutatorData),
	))

	// Room Admin
	http.HandleFunc("GET /admin/rooms/", RunWithMUDLocked(
		doBasicAuth(roomsIndex),
	))
	http.HandleFunc("GET /admin/rooms/roomdata/", RunWithMUDLocked(
		doBasicAuth(roomData),
	))

	// HTTP Server
	wg.Add(1)
	httpServer = &http.Server{Addr: fmt.Sprintf(`:%d`, webPort)}
	go func() {

		mudlog.Info("Starting http server", "webport", webPort)

		defer wg.Done()
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			mudlog.Error("Error starting web server", "error", err)
		}
	}()

	if webHttpsPort > 0 {

		filePaths := configs.GetFilePathsConfig()

		certFile := ``
		keyFile := ``

		if _, err := os.Stat(string(filePaths.HttpsCertFile)); err == nil {
			certFile = string(filePaths.HttpsCertFile)
		}
		if _, err := os.Stat(string(filePaths.HttpsKeyFile)); err == nil {
			keyFile = string(filePaths.HttpsKeyFile)
		}

		if certFile != `` && keyFile != `` {
			wg.Add(1)
			httpsServer = &http.Server{Addr: fmt.Sprintf(`:%d`, webHttpsPort)}
			go func() {

				mudlog.Info("Starting https server", "webHttpsPort", webHttpsPort)

				defer wg.Done()
				if err := httpsServer.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
					mudlog.Error("Error starting HTTPS web server", "error", err)
				}
			}()
		}
	}

}

// This wraps the handler functiojn with a game lock (mutex) to keep the mud from
// Concurrently accessing the same memory
func RunWithMUDLocked(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		util.LockMud()
		defer util.UnlockMud()

		next.ServeHTTP(w, r)
	})
}

func Shutdown() {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown failed:%+v", err)
		}
	}

	if httpsServer != nil {
		if err := httpsServer.Shutdown(ctx); err != nil {
			log.Printf("HTTPS server shutdown failed:%+v", err)
		}
	}
}

func sendError(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	if status == http.StatusNotFound {
		fmt.Fprint(w, "custom 404")
	}
}
