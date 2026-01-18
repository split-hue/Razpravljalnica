package profile

import (
	"fmt"
	"razpravljalnica/client"

	"github.com/rivo/tview"
)

// NewTopicsPage vrača tview.Primitive
func NewProfilePage(
	pages *tview.Pages,
	appClient *client.Client) tview.Primitive {

	currentUser := appClient.GetCurrentUser()
	username := "Getting your username... Come back later."
	if currentUser != nil {
		username = fmt.Sprintf("Username: %s", currentUser.Name)
	}

	list := tview.NewList()

	list.AddItem("", "", 0, nil)
	list.AddItem("YOUR PROFILE", "Great username!", 0, nil)
	list.AddItem("", "", 0, nil)
	list.AddItem(username, "", 0, nil)

	list.AddItem("Back", "Return to menu", 'b', func() {
		pages.SwitchToPage("menu")
	})

	// Prestavimo curzor na tretji item (ker imamo tri vrstice pred prvim clickable buttonom)
	curzorPos := 4
	list.SetCurrentItem(curzorPos)

	list.SetBorder(true).SetTitle(" YOUR PROFILE ")
	return list
}
