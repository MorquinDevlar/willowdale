package usercommands

import (
	"fmt"
	"math"

	"github.com/volte6/mud/buffs"
	"github.com/volte6/mud/templates"
	"github.com/volte6/mud/users"
)

func Conditions(rest string, userId int) (bool, error) {

	// Load user details
	user := users.GetByUserId(userId)
	if user == nil { // Something went wrong. User not found.
		return false, fmt.Errorf("user %d not found", userId)
	}

	type buffInfo struct {
		Name        string
		Description string
		RoundsLeft  int
	}

	afflictions := []buffInfo{}

	charBuffs := user.Character.GetBuffs()
	for _, buff := range charBuffs {

		spec := buffs.GetBuffSpec(buff.BuffId)
		totalRounds := int(math.Ceil(float64(buff.TriggersLeft) * float64(spec.RoundInterval)))

		newAffliction := buffInfo{
			Name:        spec.Name,
			Description: spec.Description,
			RoundsLeft:  totalRounds - (buff.RoundCounter),
		}

		if spec.Secret {
			newAffliction.Name = "Mysterious Affliction"
			newAffliction.Description = "Unknown"
		}

		afflictions = append(afflictions, newAffliction)
	}

	tplTxt, _ := templates.Process("character/conditions", afflictions)
	user.SendText(tplTxt)

	return true, nil
}
