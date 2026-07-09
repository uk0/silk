// Package recipe implements FameView-style 配方 (recipe) management: named
// sets of tag values that can be saved to disk, loaded back, applied
// (downloaded) to the process, and captured from live tags.
//
// Recipes are common in batch process control: an operator selects a named
// recipe ("Cola", "Diet") and downloads its setpoints to the controller. Here
// a Recipe is a tag -> value map; Apply writes each value into the tag
// registry via core.Tag.SetValue, and RW-bound tags then push the setpoint to
// the device. Capture does the reverse — it snapshots the current live values
// of a set of tags into a new named Recipe.
package recipe

import (
	"encoding/json"
	"os"

	"github.com/uk0/silk/core"
)

// Recipe is a named set of tag -> value setpoints.
type Recipe struct {
	Name   string             `json:"name"`
	Values map[string]float64 `json:"values"`
}

// Book is an ordered collection of recipes, persisted as one JSON file.
type Book struct {
	Recipes []Recipe `json:"recipes"`
}

// Add appends r, replacing in place any existing recipe with the same Name.
func (b *Book) Add(r Recipe) {
	for i := range b.Recipes {
		if b.Recipes[i].Name == r.Name {
			b.Recipes[i] = r
			return
		}
	}
	b.Recipes = append(b.Recipes, r)
}

// Get returns the recipe named name and whether it was found.
func (b *Book) Get(name string) (Recipe, bool) {
	for _, r := range b.Recipes {
		if r.Name == name {
			return r, true
		}
	}
	return Recipe{}, false
}

// List returns the names of every recipe in book order.
func (b *Book) List() []string {
	out := make([]string, 0, len(b.Recipes))
	for _, r := range b.Recipes {
		out = append(out, r.Name)
	}
	return out
}

// Delete removes the recipe named name, reporting whether one was removed.
func (b *Book) Delete(name string) bool {
	for i := range b.Recipes {
		if b.Recipes[i].Name == name {
			b.Recipes = append(b.Recipes[:i], b.Recipes[i+1:]...)
			return true
		}
	}
	return false
}

// Save writes the book to path as indented JSON.
func (b *Book) Save(path string) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads a book previously written by Save.
func Load(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b := &Book{}
	if err := json.Unmarshal(data, b); err != nil {
		return nil, err
	}
	return b, nil
}

// Apply downloads r into tags: each Values[tag] is written to its tag via
// SetValue, creating the tag on first use. RW-bound tags then push the new
// setpoint to the device.
func Apply(r Recipe, tags *core.TagDB) {
	for name, v := range r.Values {
		tags.GetOrCreate(name, core.Meta{}).SetValue(v)
	}
}

// Capture snapshots the current value of each named tag into a new Recipe.
// A tag that does not yet exist is created and captured as 0.
func Capture(name string, tags *core.TagDB, names []string) Recipe {
	vals := make(map[string]float64, len(names))
	for _, n := range names {
		vals[n] = tags.GetOrCreate(n, core.Meta{}).Value().Float()
	}
	return Recipe{Name: name, Values: vals}
}
