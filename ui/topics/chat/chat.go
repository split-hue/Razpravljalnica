package chat

import (
	"fmt"
	"razpravljalnica/client"
	"razpravljalnica/razpravljalnica"
	"sort"
	"time"

	"github.com/rivo/tview"
)

///////////////////////////
// CHAT_TOPIC PAGE
// - izpiše vse messages/comments na ta topic
// - lahko dodaš nov comment na ta topic (stran se refresha, da vidiš nov comment)
// - lahko dodaš like na comment (stran se refreha, da vidiš nov like)
// - exit na TOPIC page
///////////////////////////

type ChatPage struct {
	Root  tview.Primitive
	app   *tview.Application
	pages *tview.Pages
	cl    *client.Client

	topic *razpravljalnica.Topic

	messages      *tview.List
	buttonSection *tview.List
	form          *tview.Form
	sendBtn       *tview.Button

	lastFromMsgID int64

	lastMsgSig string
	stopPoll   chan struct{}
}

func NewChatPage(
	app *tview.Application,
	pages *tview.Pages,
	appClient *client.Client,
	topic *razpravljalnica.Topic) *ChatPage {

	p := &ChatPage{
		app:   app,
		pages: pages,
		cl:    appClient,
		topic: topic}

	// Messages
	p.messages = tview.NewList()
	p.messages.SetBorder(true).SetTitle(" CHAT ")

	// Button section
	p.buttonSection = tview.NewList()
	p.buttonSection.SetBorder(true)
	p.buttonSection.AddItem("SUBSCRIBE TO TOPIC", "Recieve live news about this topic on home page", 's', func() {
		p.SetButtonsToSubscribed()
		p.subscribeToThisTopic()
	})
	p.buttonSection.AddItem("Back", "Return to menu", 'b', func() {
		p.StopPolling() // Nehamo pridobivat podatke iz serverja za to stran
		p.pages.SwitchToPage("topics")
	})

	// Form
	p.form = tview.NewForm()
	p.form.AddInputField("Your Message: ", "", 30, nil, nil)
	p.form.AddButton("Post Message", func() {
		input := p.form.GetFormItemByLabel("Your Message: ").(*tview.InputField).GetText()
		if input == "" {
			return
		}
		p.form.GetFormItemByLabel("Your Message: ").(*tview.InputField).SetText("")

		p.PostMessageAndRefresh(input)
	})
	p.form.SetBorder(true).SetTitle(" Post Message To Topic ")

	// Vse združimo v flex
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(p.messages, 0, 5, true)
	flex.AddItem(p.buttonSection, 0, 2, false)
	flex.AddItem(p.form, 0, 2, false)
	p.Root = flex

	p.GetMessagesAndRefresh()
	return p
}

func (p *ChatPage) SetButtonsToSubscribed() {
	go func() {
		p.app.QueueUpdateDraw(func() {
			p.buttonSection.Clear()
			p.buttonSection.AddItem("ALREADY SUBSCRIBED", "Check for news on your home page", 0, nil)
			p.buttonSection.AddItem("Back", "Return to menu", 'b', func() {
				p.StopPolling()
				p.pages.SwitchToPage("topics")
			})
		})
	}()
}

// Pokliči client (subrscribe to topic)
func (p *ChatPage) subscribeToThisTopic() {
	go func() {
		p.cl.SubscribeTopic(p.topic.Id)
	}()
}

func (p *ChatPage) GetMessagesAndRefresh() {
	go func() {
		resp := p.cl.GetMessagesByTopic(p.topic.Id, 0, 0)

		p.app.QueueUpdateDraw(func() { // Nato izrišemo UI
			p.RefreshUI(resp)
		})
	}()
}

func (p *ChatPage) PostMessageAndRefresh(input string) {
	go func() {
		new_message := p.cl.PostMessageToTopic(p.topic.Id, input)
		if new_message == nil {
			return
		}

		p.app.QueueUpdateDraw(func() {
			p.addMessageToList(new_message)
		})
	}()
}

func (p *ChatPage) LikeMessageAndRefresh(messageID int64) {
	go func() {
		p.cl.LikeMessage(p.topic.Id, messageID)
		resp := p.cl.GetMessagesByTopic(p.topic.Id, 0, 0)

		p.app.QueueUpdateDraw(func() {
			p.RefreshUI(resp)
		})
	}()
}

func (p *ChatPage) RefreshUI(resp *razpravljalnica.GetMessagesResponse) {
	// ohrani selection, da UI ne skače
	cur := p.messages.GetCurrentItem()

	msgs := []*razpravljalnica.Message{}
	if resp != nil {
		msgs = resp.Messages
	}

	p.messages.Clear()

	if len(msgs) == 0 {
		p.messages.AddItem("No messages yet.", "", 0, nil)
	} else {
		sorted_msgs := []*razpravljalnica.Message{}
		if resp != nil {
			sorted_msgs = append([]*razpravljalnica.Message(nil), resp.Messages...)
		}

		sort.Slice(sorted_msgs, func(i, j int) bool {
			return sorted_msgs[i].Id < sorted_msgs[j].Id
		})

		for _, m := range sorted_msgs {
			p.addMessageToList(m)
		}
	}

	// restore selection
	if cur >= 0 && cur < p.messages.GetItemCount() {
		p.messages.SetCurrentItem(cur)
	}
}

func (p *ChatPage) addMessageToList(m *razpravljalnica.Message) {
	title := fmt.Sprintf("user: %d  ♥ %d", m.UserId, m.Likes)
	subtitle := m.Text

	p.messages.AddItem(
		title,
		subtitle,
		0,
		func() { p.LikeMessageAndRefresh(m.Id) }) // On click like message
}

func (p *ChatPage) StartPolling(interval time.Duration) {
	if p.stopPoll != nil {
		return
	}
	p.stopPoll = make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				resp := p.cl.GetMessagesByTopic(p.topic.Id, 0, 0)

				if p.dataChanged(resp) {
					p.app.QueueUpdateDraw(func() { p.RefreshUI(resp) })
				}
			case <-p.stopPoll:
				return
			}
		}
	}()
}

func (p *ChatPage) StopPolling() {
	if p.stopPoll != nil {
		close(p.stopPoll)
		p.stopPoll = nil
	}
}

// Preveri ali se je data od prejšnjega ticka spremenil
func (p *ChatPage) dataChanged(resp *razpravljalnica.GetMessagesResponse) bool {
	if resp == nil || len(resp.Messages) == 0 {
		return false
	}

	najnovejsi_message := resp.Messages[len(resp.Messages)-1]
	st_likes := int32(0)
	for _, m := range resp.Messages {
		st_likes += m.Likes
	}

	// Sestavimo data iz zadnjega sporočila, messages length in st vseh lajkov (da je zihr unikatno)
	new_data := fmt.Sprintf("%d-%d-%d", najnovejsi_message.Id, len(resp.Messages), st_likes)

	if new_data != p.lastMsgSig {
		p.lastMsgSig = new_data
		return true // data changed
	}
	return false // data not changed
}
