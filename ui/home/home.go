package home

import (
	"fmt"
	"razpravljalnica/client"
	"razpravljalnica/razpravljalnica"
	"strings"

	"github.com/rivo/tview"
)

//////////////////////
// HOME PAGE
// - posluša za subscribed topics
// - exit na menu
//////////////////////

type HomePage struct {
	Root tview.Primitive
	app  *tview.Application
	cl   *client.Client

	header              *tview.List
	listOfSubscriptions *tview.List
	buttonSection       *tview.List
	news                *tview.List
}

const MAX_NEWS = 100

func NewHomePage(
	app *tview.Application,
	pages *tview.Pages,
	appClient *client.Client) *HomePage {

	hp := &HomePage{app: app, cl: appClient}

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

	// HEADER
	hp.header = tview.NewList()
	hp.header.SetBorder(true).SetTitle(" HOME ")
	hp.header.AddItem("", "", 0, nil)
	hp.header.AddItem("WELCOME HOME!", "", 0, nil)
	hp.header.AddItem("[green]See what's new", "", 0, nil)
	hp.header.AddItem("", "", 0, nil)

	// LIST OF CURRENT SUBSCRIPTIONS
	hp.listOfSubscriptions = tview.NewList()
	hp.listOfSubscriptions.SetBorder(true).SetTitle(" SUBSCRIBED ")

	// Button section (button back)
	hp.buttonSection = tview.NewList()
	hp.buttonSection.SetBorder(true)
	hp.buttonSection.AddItem("Back", "Return to menu", 'b', func() {
		pages.SwitchToPage("menu")
	})
	/*hp.buttonSection.AddItem("Clear news", "Clear home page", 'c', func() {
		hp.ClearNews()
	})*/

	// News section
	hp.news = tview.NewList()
	hp.news.SetBorder(true).SetTitle(" NEWS ")

	///////////////////////////
	// DAMO VS lepo NA EKRAN //
	//////////////////////////
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	flex.AddItem(logo, 0, 1, false)
	flex.AddItem(hp.header, 0, 1, false)
	flex.AddItem(hp.listOfSubscriptions, 0, 2, false)
	flex.AddItem(hp.buttonSection, 0, 1, false)

	flexVertical := tview.NewFlex().SetDirection(tview.FlexColumn)
	flexVertical.AddItem(flex, 0, 2, false)
	flexVertical.AddItem(hp.news, 0, 3, false)

	hp.Root = flexVertical

	go func() {
		// Čakamo na event, ko pride, izrišemo to na home screen
		for event := range appClient.NewsEvents() {
			title := "post"
			if event.Op == razpravljalnica.OpType_OP_LIKE {
				title = "like"
			}
			news_title := ""

			if title == "post" {
				news_title = fmt.Sprintf("New MESSAGE on topic %d", event.TopicID)
			} else {
				news_title = fmt.Sprintf("New LIKE on topic %d [%d ♥]", event.TopicID, event.Likes)
			}
			message := event.Text

			app.QueueUpdateDraw(func() {
				hp.AppendNewsNewest(news_title, message)
			})
		}
	}()

	return hp
}

func (p *HomePage) RefreshSubscriptionsList() {
	go func() {
		subs := p.cl.GetSubscriptions() // najprej izvedemo klic do strežnika
		topics_list := p.cl.ListTopics()

		if subs == nil || topics_list == nil {
			return
		}

		topicsByID := make(map[int64]*razpravljalnica.Topic)
		for _, t := range topics_list.Topics {
			topicsByID[t.Id] = t
		}

		p.app.QueueUpdateDraw(func() {
			p.listOfSubscriptions.Clear()

			for _, sub := range subs {
				topic, ok := topicsByID[sub.TopicID]
				if !ok {
					continue // topic ne obstaja
				}

				title := fmt.Sprintf(
					"- %s",
					strings.ToUpper(topic.Name),
				)
				p.listOfSubscriptions.AddItem(title, "", 0, nil)
			}
		})
	}()
}

func (p *HomePage) AppendNewsNewest(title, subtitle string) {
	p.news.InsertItem(0, title, subtitle, 0, nil)

	if p.news.GetItemCount() > MAX_NEWS {
		p.news.RemoveItem(p.news.GetItemCount() - 1)
	}
}

func (p *HomePage) AppendNews(title, subtitle string) {
	p.app.QueueUpdateDraw(func() {
		p.news.AddItem(title, subtitle, 0, nil)
	})
}

func (p *HomePage) ClearNews() {
	p.app.QueueUpdateDraw(func() {
		p.news.Clear()
	})
}
