package usercommands

import (
	"fmt"

	"github.com/volte6/mud/characters"
	"github.com/volte6/mud/mobs"
	"github.com/volte6/mud/rooms"
	"github.com/volte6/mud/skills"
	"github.com/volte6/mud/users"
	"github.com/volte6/mud/util"
)

func Hire(rest string, userId int) (bool, error) {

	if rest == "" {
		return List(rest, userId)
	}

	// Load user details
	user := users.GetByUserId(userId)
	if user == nil { // Something went wrong. User not found.
		return false, fmt.Errorf(`user %d not found`, userId)
	}

	// Load current room details
	room := rooms.LoadRoom(user.Character.RoomId)
	if room == nil {
		return false, fmt.Errorf(`room %d not found`, user.Character.RoomId)
	}

	maxCharmed := user.Character.GetSkillLevel(skills.Tame) + 1

	if len(user.Character.GetCharmIds()) >= maxCharmed {
		user.SendText(fmt.Sprintf(`You can only have %d creatures following you at a time.`, maxCharmed))
		return true, nil
	}

	for _, mobId := range room.GetMobs(rooms.FindMerchant) {

		mob := mobs.GetInstance(mobId)
		if mob == nil {
			continue
		}

		mercNames := []string{}
		for _, hireInfo := range mob.ShopServants {
			if mobInfo := mobs.GetMobSpec(hireInfo.MobId); mobInfo != nil {
				mercNames = append(mercNames, mobInfo.Character.Name)
			}
		}

		match, closeMatch := util.FindMatchIn(rest, mercNames...)
		if match == "" {
			match = closeMatch
		}

		if match == "" {
			extraSay := ""
			if len(mercNames) > 0 {
				extraSay = fmt.Sprintf(` Any interest in a <ansi fg="itemname">%s</ansi>?`, mercNames[util.Rand(len(mercNames))])
			}

			mob.Command(`say Sorry, I don't have that for hire right now.` + extraSay)

			return true, nil
		}

		for idx, hireInfo := range mob.ShopServants {
			mobInfo := mobs.GetMobSpec(hireInfo.MobId)
			if mobInfo == nil {
				continue
			}
			if mobInfo.Character.Name != match {
				continue
			}

			if user.Character.Gold < hireInfo.Price {

				mob.Command(`say You don't have enough gold.`)

				return true, nil
			}

			user.Character.Gold -= hireInfo.Price
			mob.Character.Gold += hireInfo.Price >> 2 // Keeps 1/4th, the rest disappears

			hireInfo.Quantity--
			if hireInfo.Quantity <= 0 {
				// remove pos idx
				mob.ShopServants = append(mob.ShopServants[:idx], mob.ShopServants[idx+1:]...)
			} else {
				mob.ShopServants[idx] = hireInfo
			}

			newMob := mobs.NewMobById(mobInfo.MobId, user.Character.RoomId)
			// Charm 'em
			newMob.Character.Charm(user.UserId, -2, characters.CharmExpiredRevert)
			user.Character.TrackCharmed(newMob.InstanceId, true)

			room.AddMob(newMob.InstanceId)

			user.SendText(
				fmt.Sprintf(`You pay <ansi fg="gold">%d</ansi> gold to <ansi fg="mobname">%s</ansi>.`, hireInfo.Price, mob.Character.Name),
			)
			room.SendText(
				fmt.Sprintf(`<ansi fg="username">%s</ansi> pays <ansi fg="gold">%d</ansi> gold to <ansi fg="mobname">%s</ansi>.`, user.Character.Name, hireInfo.Price, mob.Character.Name),
				userId,
			)

			newMob.Command(`emote is ready to serve.`)

			break

		}
	}

	return true, nil
}
