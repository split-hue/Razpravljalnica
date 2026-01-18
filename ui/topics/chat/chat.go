package chat

import (
	"fmt"
	"razpravljalnica/client"
	"razpravljalnica/razpravljalnica"
	"strings"

	"github.com/rivo/tview"
)

///////////////////////////
// CHAT_TOPIC PAGE
// - izpiše vse messages/comments na ta topic
// - lahko dodaš nov comment na ta topic (stran se refresha, da vidiš nov comment)
// - lahko dodaš like na comment (stran se refreha, da vidiš nov like)
// - exit na TOPIC page
///////////////////////////

func NewChatPage(pages *tview.Pages,
	appClient *client.Client,
	topic *razpravljalnica.Topic,
	messageCh chan<- *razpravljalnica.Topic) tview.Primitive {

	/////////////////////////////////////////
	// CREATE LIST OF MESSAGES AND BUTTONS //
	/////////////////////////////////////////
	messages := appClient.GetMessagesByTopic(topic.Id, 0, 0) // 0, 0 for all messages with no limit
	list := tview.NewList()

	list.AddItem("", "", 0, nil)
	list.AddItem(fmt.Sprintf("[yellow]Chat about %s", topic.Name), "", 0, nil)

	if len(messages.Messages) == 0 {
		list.AddItem("Currently there are no messages. Write something!", "", 0, nil)
	} else {
		for _, msg := range messages.Messages {

			m := msg
			list.AddItem(
				fmt.Sprintf("%s [comment by user with id: %d]", m.Text, m.UserId),
				fmt.Sprintf("[black]---[red]LIKE  [pink]All likes: %d", msg.Likes),
				0,
				func() {
					go func() {
						appClient.LikeMessage(m.TopicId, m.Id)
						// Pošljemo main.go, da naj posodobi topic page
						messageCh <- topic
					}()
				},
			)
		}
	}

	// gumb back
	list.AddItem("Back", "Return to topics", 'b', func() {
		pages.SwitchToPage("topics")
		pages.HidePage(fmt.Sprintf("chat-%d", topic.Id))
	})

	///////////////////////
	// CREATE INPUT FORM //
	//////////////////////
	form := tview.NewForm()

	form.AddInputField("Your Message", "", 30, nil, nil)
	form.AddButton("Add", func() {

		inputText := form.GetFormItemByLabel("Your Message").(*tview.InputField).GetText()
		if inputText == "" {
			return
		}

		form.GetFormItemByLabel("Your Message").(*tview.InputField).SetText("")

		go func() {
			appClient.PostMessageToTopic(topic.Id, inputText)
			// Pošljemo main.go, da naj posodobi topic page
			messageCh <- topic
		}()
	})

	// Prestavimo curzor na tretji item (ker imamo tri vrstice pred prvim clickable buttonom) + st messages
	curzorPos := len(messages.Messages) + 3
	list.SetCurrentItem(curzorPos)

	///////////////////////////
	// DAMO VS lepo NA EKRAN //
	//////////////////////////
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// seznam messages
	list.SetBorder(true).SetTitle(" " + strings.ToUpper(topic.Name) + " ")
	flex.AddItem(list, 0, 3, true) //true = fokus ob startu

	// form za nov message
	form.SetBorder(true).SetTitle(" Add New Message to Topic ")
	flex.AddItem(form, 0, 1, false)

	return flex
}
