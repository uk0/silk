package gui

import (
//	"encoding/xml"
//"io/ioutil"
//"os"
//"strings"
)

/*
type ActionCategory struct {
	Parent string
	ID     string
	Name   string
	Order  int
}

type _ActionGroup struct {
	list []IActionLike
	menu *Menu
}

func (a *ActionCategory) Text() string {
	return a.Name
}

func (a *ActionCategory) DisplayOrderHint() int {
	return a.Order
}

func (a *ActionCategory) CategoryHint() string {
	return a.Parent
}

type IActionLike interface {
	Text() string
	DisplayOrderHint() int
	CategoryHint() string
}

func loadMenuConfig(path string) (ret []ActionCategory, err error) {
	file, err := os.Open(path)
	if err != nil {
		return
	}

	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return
	}

	err = xml.Unmarshal(buf, &ret)
	return
}

*/

func BuildMenu(actions []IAction, cfgFilePath string) *Menu {
	/*
		var cfg []ActionCategory
		if cfgFilePath != "" {
			cfg, _ = loadMenuConfig(cfgFilePath)
		}

		var v []IActionLike
		for _, x := range actions {
			v = append(v, x.(IActionLike))
		}
		for i, _ := range cfg {
			v = append(v, &cfg[i])
		}

		groups := make(map[string]*_ActionGroup)

		for _, a := range v {
			category := a.CategoryHint()
			order := a.DisplayOrderHint()

			p, ok := groups[category]
			if ok {
				for _, b := range p.list {
					if b.DisplayOrderHint() < order {
						continue
					}
					if b.DisplayOrderHint() == order && b.Text() < a.Text() {
						continue
					}
					p.list = append(p.list, a)
					break
				}
			} else {
				p = new(_ActionGroup)
				p.list = append(p.list, a)
				groups[category] = p
			}
		}

		for _, p := range groups {
			if p.menu == nil {
				p.menu = NewMenu(false)
			}
			for _, a := range p.list {
				switch x := a.(type) {
				case IAction:
					p.menu.AddActionButton(x)
				case *ActionCategory:
					if sub, ok := groups[x.Name]; ok {
						if sub.menu == nil {
							sub.menu = NewMenu(false)
						}
						p.menu.AddSubMenu(x.Name, nil, sub.menu)
					}
				default:
					continue
				}
			}
		}

		if root, ok := groups[""]; ok {
			return root.menu
		}
		return nil

	*/

	if len(actions) == 0 {
		return nil
	}
	menu := NewMenu(false)
	for _, a := range actions {
		menu.AddActionButton(a)
	}
	return menu
}
