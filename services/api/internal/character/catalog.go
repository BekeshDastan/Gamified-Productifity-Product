package character

import (
	"embed"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

const CatalogVersion = "cosmetics-v1"

//go:embed assets/*.svg
var Assets embed.FS

type Item struct {
	ID          string `json:"id"`
	Slot        string `json:"slot"`
	Name        string `json:"name"`
	Rarity      string `json:"rarity"`
	Starter     bool   `json:"starter"`
	Level       int64  `json:"level"`
	Achievement string `json:"achievement"`
	URL         string `json:"url"`
}

var slots = []string{"background", "base", "clothes", "head", "accessory", "frame"}

func Catalog() []Item {
	return []Item{{ID: "base-0", Slot: "base", Name: "Основа путника", Rarity: "common", Starter: true, Level: 1, Achievement: "", URL: "/api/v1/cosmetics/v1/base-0.svg"}, {ID: "clothes-0", Slot: "clothes", Name: "Одежда путника", Rarity: "common", Starter: true, Level: 1, Achievement: "", URL: "/api/v1/cosmetics/v1/clothes-0.svg"}, {ID: "head-0", Slot: "head", Name: "Головной убор путника", Rarity: "common", Starter: true, Level: 1, Achievement: "", URL: "/api/v1/cosmetics/v1/head-0.svg"}, {ID: "accessory-0", Slot: "accessory", Name: "Аксессуар путника", Rarity: "common", Starter: true, Level: 1, Achievement: "", URL: "/api/v1/cosmetics/v1/accessory-0.svg"}, {ID: "background-0", Slot: "background", Name: "Фон путника", Rarity: "common", Starter: true, Level: 1, Achievement: "", URL: "/api/v1/cosmetics/v1/background-0.svg"}, {ID: "frame-0", Slot: "frame", Name: "Рамка путника", Rarity: "common", Starter: true, Level: 1, Achievement: "", URL: "/api/v1/cosmetics/v1/frame-0.svg"}, {ID: "base-1", Slot: "base", Name: "Основа рассвета", Rarity: "uncommon", Starter: false, Level: 2, Achievement: "", URL: "/api/v1/cosmetics/v1/base-1.svg"}, {ID: "clothes-1", Slot: "clothes", Name: "Одежда рассвета", Rarity: "uncommon", Starter: false, Level: 2, Achievement: "", URL: "/api/v1/cosmetics/v1/clothes-1.svg"}, {ID: "head-1", Slot: "head", Name: "Головной убор рассвета", Rarity: "uncommon", Starter: false, Level: 2, Achievement: "", URL: "/api/v1/cosmetics/v1/head-1.svg"}, {ID: "accessory-1", Slot: "accessory", Name: "Аксессуар рассвета", Rarity: "uncommon", Starter: false, Level: 3, Achievement: "task-1", URL: "/api/v1/cosmetics/v1/accessory-1.svg"}, {ID: "background-1", Slot: "background", Name: "Фон рассвета", Rarity: "uncommon", Starter: false, Level: 3, Achievement: "", URL: "/api/v1/cosmetics/v1/background-1.svg"}, {ID: "frame-1", Slot: "frame", Name: "Рамка рассвета", Rarity: "uncommon", Starter: false, Level: 3, Achievement: "", URL: "/api/v1/cosmetics/v1/frame-1.svg"}, {ID: "base-2", Slot: "base", Name: "Основа леса", Rarity: "uncommon", Starter: false, Level: 4, Achievement: "", URL: "/api/v1/cosmetics/v1/base-2.svg"}, {ID: "clothes-2", Slot: "clothes", Name: "Одежда леса", Rarity: "uncommon", Starter: false, Level: 4, Achievement: "", URL: "/api/v1/cosmetics/v1/clothes-2.svg"}, {ID: "head-2", Slot: "head", Name: "Головной убор леса", Rarity: "uncommon", Starter: false, Level: 4, Achievement: "", URL: "/api/v1/cosmetics/v1/head-2.svg"}, {ID: "accessory-2", Slot: "accessory", Name: "Аксессуар леса", Rarity: "rare", Starter: false, Level: 5, Achievement: "", URL: "/api/v1/cosmetics/v1/accessory-2.svg"}, {ID: "background-2", Slot: "background", Name: "Фон леса", Rarity: "rare", Starter: false, Level: 5, Achievement: "", URL: "/api/v1/cosmetics/v1/background-2.svg"}, {ID: "frame-2", Slot: "frame", Name: "Рамка леса", Rarity: "rare", Starter: false, Level: 5, Achievement: "", URL: "/api/v1/cosmetics/v1/frame-2.svg"}, {ID: "base-3", Slot: "base", Name: "Основа звёзд", Rarity: "rare", Starter: false, Level: 6, Achievement: "", URL: "/api/v1/cosmetics/v1/base-3.svg"}, {ID: "clothes-3", Slot: "clothes", Name: "Одежда звёзд", Rarity: "rare", Starter: false, Level: 6, Achievement: "", URL: "/api/v1/cosmetics/v1/clothes-3.svg"}}
}
func MountAssets(r chi.Router) {
	r.Get("/api/v1/cosmetics/v1/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := chi.URLParam(r, "file")
		for _, item := range Catalog() {
			if file == item.ID+".svg" {
				b, e := Assets.ReadFile("assets/" + file)
				if e != nil {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "image/svg+xml")
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				_, _ = w.Write(b)
				return
			}
		}
		http.NotFound(w, r)
	})
}
func starterEquipment() map[string]string {
	out := map[string]string{}
	for _, i := range Catalog() {
		if i.Starter {
			out[i.Slot] = i.ID
		}
	}
	return out
}
func validSlot(slot string) bool {
	for _, s := range slots {
		if s == slot {
			return true
		}
	}
	return false
}
func itemByID(id string) (Item, bool) {
	for _, i := range Catalog() {
		if i.ID == strings.TrimSpace(id) {
			return i, true
		}
	}
	return Item{}, false
}
