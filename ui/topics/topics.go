package topics

import (
	"fmt"
	"razpravljalnica/client"
	"razpravljalnica/razpravljalnica"
	"razpravljalnica/ui/topics/chat"
	"strings"

	"github.com/rivo/tview"
)

// NewTopicsPage vrača tview.Primitive
func NewTopicsPage(
	app *tview.Application,
	pages *tview.Pages,
	appClient *client.Client,
	topicCh chan<- bool,
	messageCh chan<- *razpravljalnica.Topic) tview.Primitive {

	///////////////////////////////////////
	// CREATE LIST OF TOPICS AND BUTTONS //
	///////////////////////////////////////

	list := tview.NewList()
	list.AddItem("", "", 0, nil)
	list.AddItem("[yellow]ALL TOPICS", "(Click on a topic to open the chat)", 0, nil) // nil = neizbiran element
	list.AddItem("", "", 0, nil)

	topics := appClient.ListTopics()

	// Naloži teme iz TAIL
	if len(topics.Topics) == 0 {
		list.AddItem("Currently there are no topics. Create some!", "", 0, nil)
	} else {
		for _, topic := range topics.Topics {
			t := topic // <--- kopija za closure
			list.AddItem(
				fmt.Sprintf("- %s", strings.ToUpper(t.Name)),
				"",
				0,
				func() {
					pageName := fmt.Sprintf("chat_%d", t.Id)
					if !pages.HasPage(pageName) {
						chatPage := chat.NewChatPage(pages, appClient, t, messageCh)
						pages.AddPage(pageName, chatPage, true, true)
					} else {
						pages.ShowPage(pageName)
					}
					pages.HidePage("topics")
					pages.SwitchToPage(pageName)
				},
			)
		}

	}
	// BUTTON BACK
	list.AddItem("Back", "Return to menu", 'b', func() {
		pages.SwitchToPage("menu")
	})

	// Prestavimo curzor na tretji item (ker imamo tri vrstice pred prvim clickable buttonom)
	curzorPos := 3
	list.SetCurrentItem(curzorPos)

	///////////////////////
	// CREATE INPUT FORM //
	//////////////////////
	form := tview.NewForm()

	form.AddInputField("New Topic", "", 30, nil, nil)
	form.AddButton("Add", func() {

		inputText := form.GetFormItemByLabel("New Topic").(*tview.InputField).GetText()
		if inputText == "" {
			return
		}
		// Clear input field
		form.GetFormItemByLabel("New Topic").(*tview.InputField).SetText("")

		go func() {
			// SEND CREARE TOPIC REQZEST TO HEAD
			appClient.CreateTopic(inputText)
			topicCh <- true
		}()
	})

	///////////////////////////
	// DAMO VS lepo NA EKRAN //
	//////////////////////////
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// seznam tem
	list.SetBorder(true).SetTitle(" TOPICS ")
	flex.AddItem(list, 0, 3, true) //true = fokus ob startu

	// form za novo temo
	form.SetBorder(true).SetTitle(" Add New Topic ")
	flex.AddItem(form, 0, 1, false)

	return flex
}
