package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"text/template"

	socketIO "github.com/geir54/goSocketIOClient"

	pushjet "github.com/geir54/goPushJet"
)

var dataStore map[string]pushjet.Service
var datamutex sync.Mutex

type data struct {
	QRlink string
}

func handler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
	node := r.URL.Query().Get("node")

	datamutex.Lock()
	_, ok := dataStore[node]
	datamutex.Unlock()

	if ok {
		fmt.Fprintf(w, "Node already exists")
		return
	}

	service, err := pushjet.CreateService(node, "")
	if err != nil {
		log.Println(err)
		return
	}

	datamutex.Lock()
	dataStore[node] = service
	datamutex.Unlock()

	QRlink := service.GetQR()

	t := template.New("create.html")
	t, err = t.ParseFiles("create.html")
	if err != nil {
		log.Fatal(err)
	}
	p := data{QRlink: QRlink}
	err = t.Execute(w, p)
	if err != nil {
		log.Fatal(err)
	}
}

type uploadRow struct {
	I  int    `json: "i"`  // database ID
	Ni int    `json: "ni"` // serial number
	T  string `json: "t"`  // timestamp
	P  string `json: "p"`  // packet
	S  string `json: "s"`  // status
	R  int    `json: "r"`  // RSSI as received by the gateway
}

type packet struct {
	NodeID string
}

func (data *uploadRow) parse() packet {
	p := packet{}
	p.NodeID = data.P[strings.Index(data.P, "[")+1 : len(data.P)-1]
	if strings.Index(p.NodeID, ",") > 0 {
		p.NodeID = p.NodeID[:strings.Index(p.NodeID, ",")]
	}

	return p
}

func parseWorker(input chan string) {
	for {
		incomming := <-input
		data := uploadRow{}
		err := json.Unmarshal([]byte(incomming), &data)
		if err != nil {
			log.Println(err)
		}

		nodeID := data.parse().NodeID
		fmt.Println(data.P + "    " + nodeID)

		datamutex.Lock()
		service, ok := dataStore[nodeID]
		secret := service.Secret
		datamutex.Unlock()

		if ok {
			err := pushjet.SendMessage(
				secret,
				"Your node just sendt something",
				"UKHASnet.",
				5, "")
			if err != nil {
				panic(err)
			}
		}

	}
}

func ukhasListener(channel chan string) {
	conn, err := socketIO.Dial("https://ukhas.net/logtail")
	if err != nil {
		log.Fatal(err)
	}

	for {
		msg := <-conn.Output
		if msg.Event == "upload_row" {
			channel <- msg.Data
		}
	}
}

func main() {
	channel := make(chan string, 10)
	go parseWorker(channel)
	go ukhasListener(channel)

	dataStore = make(map[string]pushjet.Service)

	http.HandleFunc("/", handler)
	http.HandleFunc("/create", handleCreate)
	http.ListenAndServe(":8080", nil)
}
