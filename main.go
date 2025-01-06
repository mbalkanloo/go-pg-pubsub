package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func main() {
	// flags
	connString := flag.String("conn", "postgresql://postgres@localhost:5432", "db connection string")
	listenPort := flag.String("port", "80", "listen port")
	chanString := flag.String("chan", "", "comma-separated list of postgresql channels (ex. foo,bar)")

	flag.Parse()

	// connect to database
	log.Println("connecting to database at", *connString)
	db, err := pgx.Connect(context.Background(), *connString)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close(context.Background())

	// listen for notifications
	// communicate notifications via channel
	channels := strings.Split(*chanString, ",")
	notifications := make(chan *pgconn.Notification)
	go ListenForNotifications(db, channels, notifications)

	// listen for subscriptions and upgrade to websockets
	subscriptions := make(map[string][]*websocket.Conn)
	router := mux.NewRouter()
	router.HandleFunc("/subscribe/{id}", Subscribe(subscriptions)).Methods("GET")
	websocketServer := &http.Server{Handler:router,Addr:":" + *listenPort}
	// wrap ListenAndServe to handle errors
	go WrapServer(websocketServer)

	// handle signals by closing websocket connections and exiting
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go HandleSignal(sig, db, subscriptions)

	// publish notifications received from channel
	log.Println("publishing notifications")
	PublishNotifications(notifications, subscriptions)
}

func ListenForNotifications(db *pgx.Conn, channels []string, pub chan *pgconn.Notification) {
	for _, channel := range channels {
		log.Println("listening for notifications on channel", channel)
		_, err := db.Exec(context.Background(), "listen " + channel)
		if err != nil {
			log.Panic(err)
		}
	}
	for {
		note, err := db.WaitForNotification(context.Background())
		if err != nil {
			log.Println(err)
		}
		pub <-note
	}
}

func WrapServer(websocketServer *http.Server) {
	log.Println("listening for requests on", websocketServer.Addr)
	err := websocketServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func Subscribe(subscriptions map[string][]*websocket.Conn) func(http.ResponseWriter, *http.Request) {
	upgrader := websocket.Upgrader{}
	return func(w http.ResponseWriter, r *http.Request){
		// NOTE go1.22 support https://pkg.go.dev/net/http#Request.PathValue
		vars := mux.Vars(r)
		id := vars["id"]
		_, ok := subscriptions[id]
		if !ok {
			subscriptions[id] = []*websocket.Conn{}
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Panic(err)
			return
		}
		subscriptions[id] = append(subscriptions[id], conn)
	}
}

func HandleSignal(sig chan os.Signal, db *pgx.Conn, subscriptions map[string][]*websocket.Conn){
	s := <-sig
	log.Println("received signal", s)
	log.Println("closing database connection")
	err := db.Close(context.Background())
	if err != nil {
		log.Println(err)
	}
	log.Println("closing subscription connections")
	for _, conns := range subscriptions {
		for _, conn := range conns {
			err = conn.Close()
			if err != nil {
				log.Println(err)
			}
		}
	}
	os.Exit(0)
}

func PublishNotifications(notifications chan *pgconn.Notification, subscriptions map[string][]*websocket.Conn){
	for {
		note := <-notifications
		conns, ok := subscriptions[note.Channel]
		if !ok {
			continue
		}
		for i, conn := range conns {
			err := conn.WriteMessage(1, []byte(note.Payload))
			if err != nil {
				// remove connection from subscriptions
				subs := subscriptions[note.Channel]
				subs = append(subs[:i], subs[i+1:]...)
				subscriptions[note.Channel] = subs
			}
		}
	}
}
