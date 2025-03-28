package hooks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/volte6/gomud/internal/connections"
	"github.com/volte6/gomud/internal/events"
	"github.com/volte6/gomud/internal/mapper"
	"github.com/volte6/gomud/internal/mudlog"
	"github.com/volte6/gomud/internal/rooms"
	"github.com/volte6/gomud/internal/users"
)

//
// RoomChangeHandler waits for RoomChange events
// It then sends out GMCP data updates
// Also sends music changes out
//

func LocationGMCPUpdates(e events.Event) events.ListenerReturn {

	evt, typeOk := e.(events.RoomChange)
	if !typeOk {
		mudlog.Error("Event", "Expected Type", "RoomChange", "Actual Type", e.Type())
		return events.Cancel
	}

	// If this isn't a user changing rooms, just pass it along.
	if evt.UserId == 0 {
		return events.Continue
	}

	// Get user... Make sure they still exist too.
	user := users.GetByUserId(evt.UserId)
	if user == nil {
		return events.Cancel
	}

	// Get the new room data... abort if doesn't exist.
	newRoom := rooms.LoadRoom(evt.ToRoomId)
	if newRoom == nil {
		return events.Cancel
	}

	// Get the old room data... abort if doesn't exist.
	oldRoom := rooms.LoadRoom(evt.FromRoomId)
	if oldRoom == nil {
		return events.Cancel
	}

	//
	// Send GMCP Updates
	//
	if connections.GetClientSettings(user.ConnectionId()).GmcpEnabled(`Room`) {

		newRoomPlayers := strings.Builder{}

		// Send to everyone in the new room that a player arrived
		for _, uid := range newRoom.GetPlayers() {

			if uid == user.UserId {
				continue
			}

			if u := users.GetByUserId(uid); u != nil {

				if newRoomPlayers.Len() > 0 {
					newRoomPlayers.WriteString(`, `)
				}

				newRoomPlayers.WriteString(`"` + u.Character.Name + `": ` + `"` + u.Character.Name + `"`)

				if connections.GetClientSettings(u.ConnectionId()).GmcpEnabled(`Room`) {

					events.AddToQueue(events.GMCPOut{
						UserId:  uid,
						Payload: fmt.Sprintf(`Room.AddPlayer {"name": "%s", "fullname": "%s"}`, user.Character.Name, user.Character.Name),
					})

				}
			}
		}

		// Send to everyone in the old room that a player left
		for _, uid := range oldRoom.GetPlayers() {

			if uid == user.UserId {
				continue
			}

			if u := users.GetByUserId(uid); u != nil {
				if connections.GetClientSettings(u.ConnectionId()).GmcpEnabled(`Room`) {
					events.AddToQueue(events.GMCPOut{
						UserId:  uid,
						Payload: fmt.Sprintf(`Room.RemovePlayer "%s"`, user.Character.Name),
					})
				}
			}
		}

		roomInfoStr := strings.Builder{}
		roomInfoStr.WriteString(`{ `)
		roomInfoStr.WriteString(`"num": ` + strconv.Itoa(newRoom.RoomId) + `, `)
		roomInfoStr.WriteString(`"name": "` + newRoom.Title + `", `)
		roomInfoStr.WriteString(`"area": "` + newRoom.Zone + `", `)
		roomInfoStr.WriteString(`"environment": "` + newRoom.GetBiome().Name() + `", `)

		// build coords
		// room coordinates (string of numbers separated by commas - area,X,Y,Z)
		// GoMud doesn't use numbers for areas, so will use string.
		roomInfoStr.WriteString(`"coords": "` + newRoom.Zone + `, `)
		m := mapper.GetZoneMapper(newRoom.Zone)
		x, y, z, err := m.GetCoordinates(newRoom.RoomId)
		if err != nil {
			roomInfoStr.WriteString(`999999999999999999,999999999999999999,999999999999999999`)
		} else {
			roomInfoStr.WriteString(strconv.Itoa(x))
			roomInfoStr.WriteString(`, `)
			roomInfoStr.WriteString(strconv.Itoa(y))
			roomInfoStr.WriteString(`, `)
			roomInfoStr.WriteString(strconv.Itoa(z))
		}
		roomInfoStr.WriteString(`", `)
		// end coords

		// build exits
		roomInfoStr.WriteString(`"exits": {`)
		exitCt := 0
		for name, exitInfo := range newRoom.Exits {
			if exitInfo.Secret {
				continue
			}

			if !mapper.IsCompassDirection(name) {
				continue
			}

			if exitCt > 0 {
				roomInfoStr.WriteString(`, `)
			}

			roomInfoStr.WriteString(`"` + name + `": ` + strconv.Itoa(exitInfo.RoomId))

			exitCt++
		}
		roomInfoStr.WriteString(`}, `)
		// End exits

		// build exits "extra" (exits that map to roomId AND include x/y/z delta information)
		roomInfoStr.WriteString(`"exitsv2": {`)
		exitCt = 0
		deltaX, deltaY, deltaZ := 0, 0, 0
		for name, exitInfo := range newRoom.Exits {

			if exitInfo.Secret {
				continue
			}

			if exitCt > 0 {
				roomInfoStr.WriteString(`, `)
			}

			if len(exitInfo.MapDirection) > 0 {
				deltaX, deltaY, deltaZ = mapper.GetDelta(exitInfo.MapDirection)
			} else {
				deltaX, deltaY, deltaZ = mapper.GetDelta(name)
			}

			roomInfoStr.WriteString(`"` + name + `": { "num": `)
			roomInfoStr.WriteString(strconv.Itoa(exitInfo.RoomId))
			roomInfoStr.WriteString(`, "dx": `)
			roomInfoStr.WriteString(strconv.Itoa(deltaX))
			roomInfoStr.WriteString(`, "dy": `)
			roomInfoStr.WriteString(strconv.Itoa(deltaY))
			roomInfoStr.WriteString(`, "dz": `)
			roomInfoStr.WriteString(strconv.Itoa(deltaZ))

			if exitInfo.HasLock() {
				roomInfoStr.WriteString(`, "locked": true`)

				lockId := fmt.Sprintf(`%d-%s`, newRoom.RoomId, name)

				hasKey, hasSequence := user.Character.HasKey(lockId, int(exitInfo.Lock.Difficulty))

				roomInfoStr.WriteString(`, "haskey": `)
				roomInfoStr.WriteString(strconv.FormatBool(hasKey))

				roomInfoStr.WriteString(`, "haspickcombo": `)
				roomInfoStr.WriteString(strconv.FormatBool(hasSequence))
			}

			roomInfoStr.WriteString(`}`)

			exitCt++
		}
		roomInfoStr.WriteString(`}, `)
		// End exits

		// build details
		roomInfoStr.WriteString(`"details": [`)

		detailCt := 0
		if len(newRoom.GetMobs(rooms.FindMerchant)) > 0 || len(newRoom.GetPlayers(rooms.FindMerchant)) > 0 {
			if detailCt > 0 {
				roomInfoStr.WriteString(`, `)
			}
			detailCt++
			roomInfoStr.WriteString(`"shop"`)
		}
		if len(newRoom.SkillTraining) > 0 {
			if detailCt > 0 {
				roomInfoStr.WriteString(`, `)
			}
			detailCt++
			roomInfoStr.WriteString(`"trainer"`)
		}
		if newRoom.IsBank {
			if detailCt > 0 {
				roomInfoStr.WriteString(`, `)
			}
			detailCt++
			roomInfoStr.WriteString(`"bank"`)
		}
		if newRoom.IsStorage {
			if detailCt > 0 {
				roomInfoStr.WriteString(`, `)
			}
			detailCt++
			roomInfoStr.WriteString(`"storage"`)
		}
		roomInfoStr.WriteString(`]`)
		// end details

		roomInfoStr.WriteString(` }`)
		// End room info

		// send big 'ol room info object
		events.AddToQueue(events.GMCPOut{
			UserId:  user.UserId,
			Payload: "Room.Info " + roomInfoStr.String(),
		})

		// send player list for room
		events.AddToQueue(events.GMCPOut{
			UserId:  user.UserId,
			Payload: "Room.Players {" + newRoomPlayers.String() + `}`,
		})
	}

	return events.Continue
}
