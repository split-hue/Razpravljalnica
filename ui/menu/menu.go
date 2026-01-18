package menu

import (
	"razpravljalnica/client"
	"razpravljalnica/ui/profile"

	"github.com/rivo/tview"
)

// NewMenuPage vrača tview.Primitive
func NewMenuPage(pages *tview.Pages, quitCh chan bool) tview.Primitive {
	list := tview.NewList().
		AddItem("", "", 0, nil).
		AddItem("WELCOME TO RAZPRAVLJALNICA", "Select an action", 0, nil).
		AddItem("", "", 0, nil).
		AddItem("Profile", "", 'p', func() {
			pages.SwitchToPage("profile") // preklopi na profile stran
		}).
		AddItem("Home", "", 'h', func() {
			pages.SwitchToPage("home") // preklopi na profile stran
		}).
		AddItem("Browse Topics", "", 't', func() {
			pages.SwitchToPage("topics") // preklopi na topics stran
		}).
		AddItem("Quit", "Exit app", 'q', func() {
			quitCh <- true // Pošljemo v channel quitCh, da želimo da se aplikacija zapre
		})

	// Prestavimo curzor na tretji item (ker imamo tri vrstice pred prvim clickable buttonom)
	curzorPos := 3
	list.SetCurrentItem(curzorPos)

	list.SetBorder(true).SetTitle(" RAZPRAVLJALNICA ")
	return list
}

func NewLoginPage(pages *tview.Pages, appClient *client.Client) tview.Primitive {

	//currentUser := appClient.GetCurrentUser()

	///////////////////////
	// CREATE LOGIN FORM //
	//////////////////////
	form := tview.NewForm()

	logo := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(`[lavender]
     /\_/\  
    ( o.o ) 
          > ^ <      
    [yellow]RAZPRAVLJALNICA
`)

	form.AddInputField("Username", "", 30, nil, nil)
	form.AddPasswordField("Password", "", 30, '*', nil)
	form.AddButton("Login", func() {

		inputText := form.GetFormItemByLabel("Username").(*tview.InputField).GetText()
		if inputText == "" {
			return
		}
		// Clear input field
		form.GetFormItemByLabel("Username").(*tview.InputField).SetText("")

		go func() {
			// SEND CREARE TOPIC REQZEST TO HEAD
			appClient.CreateUser(inputText)

			profilePage := profile.NewProfilePage(pages, appClient)
			pages.AddPage("profile", profilePage, true, false) // profile stran je skrita
		}()

		pages.SwitchToPage("menu")
	})

	///////////////////////////
	// DAMO VS lepo NA EKRAN //
	//////////////////////////
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	flex.AddItem(logo, 7, 1, false)
	form.SetBorder(true).SetTitle(" LOGIN ")
	flex.AddItem(form, 0, 3, false)
	flex.AddItem(nil, 0, 1, false)

	return flex
}
