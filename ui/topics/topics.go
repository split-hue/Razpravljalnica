package topics

import (
	"fmt"
	"razpravljalnica/client"
	"razpravljalnica/razpravljalnica"
	"razpravljalnica/ui/topics/chat"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rivo/tview"
)

//////////////////////
// TOPICS page
// - izpiše vse topics
// - lahko dodaš nov topic (po tem se stran posodobi, da prikaže nov topic)
// - ko klikneš na topic greš v CHAT_TOPIC
// - exit na menu
/////////////////////

type TopicsPage struct {
	Root tview.Primitive // page
	app  *tview.Application
	cl   *client.Client

	pages     *tview.Pages
	chatPages map[int64]*chat.ChatPage

	listOfTopics  *tview.List
	buttonSection *tview.List
	form          *tview.Form

	lastTopicsSig string
	stopPoll      chan struct{}
}

func NewTopicsPage(
	app *tview.Application,
	pages *tview.Pages,
	appClient *client.Client) *TopicsPage {

	p := &TopicsPage{
		app:       app,
		pages:     pages,
		cl:        appClient,
		chatPages: make(map[int64]*chat.ChatPage)}

	// List of topics (samo inicializacija, napolnemo na koncu)
	p.listOfTopics = tview.NewList()
	p.listOfTopics.SetBorder(true).SetTitle(" TOPICS ")

	// Button section
	p.buttonSection = tview.NewList()
	p.buttonSection.SetBorder(true)
	p.buttonSection.AddItem("Back", "Return to menu", 'b', func() {
		p.StopPolling()
		p.pages.SwitchToPage("menu")
	})

	// Form
	p.form = tview.NewForm()
	p.form.AddInputField("New Topic: ", "", 30, nil, nil)
	p.form.AddButton("Add", func() {
		input := p.form.GetFormItemByLabel("New Topic: ").(*tview.InputField).GetText()
		if input == "" {
			return
		}
		p.form.GetFormItemByLabel("New Topic: ").(*tview.InputField).SetText("")

		p.CreateTopicRefreshList(input)
	})
	p.form.SetBorder(true).SetTitle(" Add New Topic ")

	// Vse združimo v flex
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(p.listOfTopics, 0, 5, true)
	flex.AddItem(p.buttonSection, 0, 1, false)
	flex.AddItem(p.form, 0, 2, false)
	p.Root = flex

	p.ListTopicsAndRefresh() // update the page
	return p
}

// LIST TOPICS
func (p *TopicsPage) ListTopicsAndRefresh() {
	go func() {
		resp := p.cl.ListTopics() // najprej izvedemo klic do strežnika

		// Nato izrišemo UI
		p.app.QueueUpdateDraw(func() {
			p.RefreshUI(resp)
		})
	}()
}

// CREATE TOPIC
func (p *TopicsPage) CreateTopicRefreshList(input string) {
	// Ne kličemo celotnega RefreshUI, samo dodamo newTopic na list
	go func() {
		newTopic := p.cl.CreateTopic(input)
		if newTopic == nil { // error creatanja topica
			return
		}

		p.app.QueueUpdateDraw(func() {
			p.addTopicToList(newTopic)
		})
	}()
}

func (p *TopicsPage) RefreshUI(resp *razpravljalnica.ListTopicsResponse) {
	cur := p.listOfTopics.GetCurrentItem() // save current curzor poz
	p.listOfTopics.Clear()

	// HEADER
	p.listOfTopics.AddItem("", "", 0, nil)
	p.listOfTopics.AddItem("[yellow]ALL TOPICS", "(Click on a topic to open the chat)", 0, nil)
	p.listOfTopics.AddItem("", "", 0, nil)

	if resp == nil || len(resp.Topics) == 0 {
		p.listOfTopics.AddItem("Currently there are no topics. Create some!", "", 0, nil)
	} else {
		// Sortiramo response in dodamo topics v p.list
		sorted_topics := append([]*razpravljalnica.Topic(nil), resp.Topics...)
		sort.Slice(sorted_topics, func(i, j int) bool {
			return sorted_topics[i].Id < sorted_topics[j].Id
		})

		for _, topic := range sorted_topics {
			t := topic
			p.addTopicToList(t)
		}
	}

	if cur < 3 {
		cur = 3
	}
	if cur >= p.listOfTopics.GetItemCount() {
		cur = p.listOfTopics.GetItemCount() - 1
	}
	p.listOfTopics.SetCurrentItem(cur)
}

func (p *TopicsPage) addTopicToList(t *razpravljalnica.Topic) {
	p.listOfTopics.AddItem(
		fmt.Sprintf("- %s", strings.ToUpper(t.Name)),
		"",
		0,
		func() {
			pageName := fmt.Sprintf("chat_%d", t.Id)
			var chatPage *chat.ChatPage

			if existing, ok := p.chatPages[t.Id]; ok {
				chatPage = existing
			} else {
				chatPage = chat.NewChatPage(p.app, p.pages, p.cl, t)
				p.chatPages[t.Id] = chatPage
				p.pages.AddPage(pageName, chatPage.Root, true, false)
			}

			//p.StopPolling()
			p.pages.SwitchToPage(pageName)
			chatPage.StartPolling(500 * time.Millisecond) // Začnemo pridobivat podatke iz serverja
		})
}

func (p *TopicsPage) StartPolling(interval time.Duration) {
	if p.stopPoll != nil {
		return
	} // že laufa
	p.stopPoll = make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				resp := p.cl.ListTopics()

				if p.dataChanged(resp) {
					p.app.QueueUpdateDraw(func() { p.RefreshUI(resp) })
				}
			case <-p.stopPoll:
				return
			}
		}
	}()
}

func (p *TopicsPage) StopPolling() {
	if p.stopPoll != nil {
		close(p.stopPoll)
		p.stopPoll = nil
	}
}

// Preveri ali se je data od prejšnjega ticka spremenil
func (p *TopicsPage) dataChanged(resp *razpravljalnica.ListTopicsResponse) bool {
	if resp == nil {
		return false // data NOT changed
	}

	// Iz response-a vzame samo id-je
	ids := make([]int64, 0, len(resp.Topics))
	for _, t := range resp.Topics {
		ids = append(ids, t.Id)
	}
	slices.Sort(ids) // sortira, tako da vedno vidimo, ali je isto (če bi bil drug vrstni red bi vrnilo da je drugače)

	// Zgradimo unique string "id-id-id-"
	new_data := strings.Builder{}
	for _, id := range ids {
		fmt.Fprintf(&new_data, "%d-", id)
	}

	if new_data.String() != p.lastTopicsSig {
		p.lastTopicsSig = new_data.String()
		return true // DATA CHANGED
	}
	return false // data NOT changed
}
