package main

import (
	"flag"
	"fmt"
	"log"
	"razpravljalnica/client"
	"razpravljalnica/ui/home"
	"razpravljalnica/ui/menu"
	"razpravljalnica/ui/topics"

	"github.com/rivo/tview"
)

//////////////////////////
// Začetni file za UI, ki
// - generira nov instance clienta
// - ustvari strani
// - pokliče file menu
/////////////////////////

var app *tview.Application
var pages *tview.Pages

// Kanali
var quitCh = make(chan bool)

func main() {
	// argumenti
	head := flag.String("head", "", "HEAD address")
	tail := flag.String("tail", "", "TAIL address")
	flag.Parse()

	appClient, err := client.NewClient(*head, *tail)
	if err != nil {
		log.Fatal(err)
	}

	if *head == "" || *tail == "" {
		log.Fatal("Please provide -head and -tail arguments")
	}

	fmt.Printf("HEAD: %s, TAIL: %s\n", *head, *tail)

	app = tview.NewApplication()
	app.EnableMouse(true)
	pages = tview.NewPages()

	// CHANNEL ZA IZHOD IZ APLIKACIJE
	go func() {
		<-quitCh   // čaka na signal iz menu
		app.Stop() // zapre aplikacijo
	}()

	// ustvari strani
	homePage := home.NewHomePage(app, pages, appClient)
	topicsPage := topics.NewTopicsPage(app, pages, appClient)

	menuPage := menu.NewMenuPage(pages, quitCh, topicsPage, homePage)
	loginPage := menu.NewLoginPage(pages, appClient)

	pages.AddPage("login", loginPage, true, true) // pokaži login na startu
	pages.AddPage("menu", menuPage, true, false)
	pages.AddPage("home", homePage.Root, true, false)
	pages.AddPage("topics", topicsPage.Root, true, false)

	// pokliči menu
	app.SetRoot(pages, true)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
