package view

import (
	"flux/src/utils"

	"github.com/anirban1809/tuix/tuix"
)

func Menu(
	props tuix.Props,
	onChange func(selectedItem string, index int),
	onFocusChange func(index int),
) tuix.Element {
	focussedIndex, setFocussedIndex := tuix.UseState(0)
	items := props.Get("items").([]string)

	visible := props.Get("visible").(bool)

	viewSize := 4

	if v := props.Get("viewSize"); v != nil {
		viewSize = v.(int)
	}

	viewFloor, setViewFloor := tuix.UseState(0)
	viewCeil := min(viewFloor+viewSize, len(items))

	var onAutoComplete func(string)
	if cb, ok := props.Get("onAutoComplete").(func(string)); ok {
		onAutoComplete = cb
	}

	if visible {
		if tuix.CurrentKey.Code == tuix.KeyEnter && onChange != nil {
			if len(items) > 0 {
				onChange(items[focussedIndex], focussedIndex)
			} else {
				onChange("", 0)
			}
		}

		if tuix.CurrentKey.Code == tuix.KeyTab && onAutoComplete != nil &&
			len(items) > 0 && focussedIndex < len(items) {
			onAutoComplete(items[focussedIndex])
		}

		if tuix.CurrentKey.Code == tuix.KeyUp && focussedIndex > 0 {
			next := focussedIndex - 1
			setFocussedIndex(next)
			if onFocusChange != nil {
				onFocusChange(next)
			}
			if next < viewFloor {
				setViewFloor(viewFloor - 1)
			}
		}
		if tuix.CurrentKey.Code == tuix.KeyDown &&
			focussedIndex < len(items)-1 {
			next := focussedIndex + 1
			setFocussedIndex(next)
			if onFocusChange != nil {
				onFocusChange(next)
			}
			if next >= viewFloor+viewSize {
				setViewFloor(viewFloor + 1)
			}
		}
	}

	if !visible {
		return tuix.Box(tuix.Props{}, tuix.NewStyle())
	}

	menuItems := utils.Map(
		items[viewFloor:viewCeil],
		func(item string, index int) tuix.Element {
			style := tuix.NewStyle().Foreground(tuix.Hex("#cbcbcb"))
			absIndex := viewFloor + index

			if absIndex == focussedIndex {
				style = style.Foreground(tuix.Hex("#019176"))
			}

			return tuix.Box(
				tuix.Props{Direction: tuix.Row, Gap: 1},
				tuix.NewStyle(),
				tuix.Text(item, style),
			)
		},
	)

	return tuix.Box(
		tuix.Props{Direction: tuix.Column, Padding: [4]int{0, 1, 1, 1}},
		tuix.NewStyle(),
		menuItems...,
	)
}
