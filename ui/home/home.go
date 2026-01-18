package home

import (
	"razpravljalnica/client"

	"github.com/rivo/tview"
)

//////////////////////
// HOME PAGE
// - posluša za subscribed topics
// - exit na menu
//////////////////////

// NewMenuPage vrača tview.Primitive
func NewHomePage(
	pages *tview.Pages,
	appClient *client.Client) tview.Primitive {

	logo := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(`[lavender]
   /\_/\  
  ( o.o ) 
       > ^ <      
 [yellow]RAZPRAVLJALNICA`)

	///////////////////////
	// CREATE HOME PAGE //
	//////////////////////
	list := tview.NewList()
	news := tview.NewList()
	news.SetBorder(true).SetTitle(" NEWS ")

	list.AddItem("", "", 0, nil)
	list.AddItem("WELCOME HOME!", "", 0, nil)
	list.AddItem("[green]See what's new on", "", 0, nil)
	list.AddItem("[green]subscribed topics", "", 0, nil)
	list.AddItem("", "", 0, nil)

	list.AddItem("Back", "Return to menu", 'b', func() {
		pages.SwitchToPage("menu")
	})

	// Prestavimo curzor na tretji item (ker imamo tri vrstice pred prvim clickable buttonom)
	curzorPos := 4
	list.SetCurrentItem(curzorPos)

	///////////////////////////
	// DAMO VS lepo NA EKRAN //
	//////////////////////////
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	flex.AddItem(nil, 0, 1, false)
	flex.AddItem(logo, 0, 1, false)
	flex.SetBorder(true).SetTitle(" HOME ")
	flex.AddItem(list, 0, 3, false)

	flexVertical := tview.NewFlex().SetDirection(tview.FlexColumn)
	flexVertical.AddItem(flex, 0, 2, false)
	flexVertical.AddItem(news, 0, 3, false)

	return flexVertical
}
