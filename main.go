package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	"sync"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}


type Client struct {
	conn     *websocket.Conn
	username string
	room     string
	mu       sync.Mutex 
}

var clients = make(map[*Client]bool)
var broadcast = make(chan Message)
var clientsMu sync.Mutex


type Message struct {
	Username string   `json:"username"`
	Content  string   `json:"content"`
	Room     string   `json:"room"`
	Users    []string `json:"users,omitempty"`
}

var RoomClientsMap = make(map[string][]*Client)

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/", homeHandler)
	router.HandleFunc("/ws/{room}/{username}", wsHandler)
	router.HandleFunc("/rooms", roomListHandler) 
	go handleMessages()

	log.Fatal(http.ListenAndServe(":8081", router))

}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

var Rooms []string


func roomListHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(Rooms)
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	room := params["room"]
	username := params["username"]
	
    if !contains(Rooms, room) {
        Rooms = append(Rooms, room)
    }

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
		return
	}

	client := &Client{conn: conn, username: username, room: room}

	clientsMu.Lock()
	clients[client] = true
	clientsMu.Unlock()

	broadcast <- Message{Username: "Server", Content: username + " joined the room.", Room: room}

	addClientToRoom(client)

	sendUsersInRoom(client)

	go client.readMessages()
}

func addClientToRoom(client *Client) {
	client.mu.Lock()
	defer client.mu.Unlock()

	RoomClientsMap[client.room] = append(RoomClientsMap[client.room], client)
}



func sendUsersInRoom(client *Client) {
	client.mu.Lock()
	defer client.mu.Unlock()

	var usersInRoom []string
	for _, c := range RoomClientsMap[client.room] {
		usersInRoom = append(usersInRoom, c.username)
	}

	client.conn.WriteJSON(Message{Username: "Server", Content: "New Users Above", Room: client.room, Users: usersInRoom})
}


func (c *Client) readMessages() {
	defer func() {
		c.conn.Close()
		delete(clients, c)

		removeClientFromRoom(c)
	}()	

	for {
		var msg Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			log.Println(err)
			break
		}

		broadcast <- msg
	}
}


func sendMessage(msg Message, client *Client) {
	client.mu.Lock()
	defer client.mu.Unlock()

	err := client.conn.WriteJSON(msg)
	if err != nil {
		log.Println(err)
		client.conn.Close()

		clientsMu.Lock()
		delete(clients, client)
		clientsMu.Unlock()

		removeClientFromRoom(client)
	}
}

func removeClientFromRoom(client *Client) {
	client.mu.Lock()
	defer client.mu.Unlock()

	roomClients, ok := RoomClientsMap[client.room]
	if ok {
		var updatedRoomClients []*Client
		for _, c := range roomClients {
			if c != client {
				updatedRoomClients = append(updatedRoomClients, c)
			}
		}
		RoomClientsMap[client.room] = updatedRoomClients
	}
}

func handleMessages() {
    for {
        msg := <-broadcast

        for client := range clients {
            if client.room == msg.Room {
                sendMessage(msg, client)
            }
        }
    }
}