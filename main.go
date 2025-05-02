package main

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	openai "github.com/sashabaranov/go-openai"
)

// ---- MODELLER --------------------------------------------------------------

type Client struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	room *Room
}

type Room struct {
	id        string
	clients   map[string]*Client
	lock      sync.RWMutex
	topic     string
	aiID      string
	started   bool
	broadcast chan []byte
}

var (
	upgrader    = websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	rooms       = make(map[string]*Room) // roomID → *Room
	roomsLock   sync.RWMutex
	openaiKey   = os.Getenv("OPENAI_API_KEY")
	openaiCli   = openai.NewClient(openaiKey)
	topics      = []string{"Yapay zekânın geleceği", "Mars kolonisi", "Türk mutfağı", "Sürdürülebilir enerji"}
	maxClients  = 2
	aiJoinDelay = 2 * time.Second
)

// ---- WEBSOCKET HANDLER ------------------------------------------------------

func wsHandler(w http.ResponseWriter, r *http.Request) {
	// 1) yükselt
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	// 2) basit kimlik: query ?name=
	name := r.URL.Query().Get("name")
	if name == "" {
		name = randomID("u")
	}
	c := &Client{id: name, conn: conn, send: make(chan []byte, 8)}
	go c.writer()
	c.reader() // bloklar
}

// ---- CLIENT I/O -------------------------------------------------------------

func (c *Client) reader() {
	defer func() {
		if c.room != nil {
			c.room.removeClient(c.id)
		}
		c.conn.Close()
	}()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var in struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(msg, &in); err != nil {
			continue
		}

		switch in.Type {

		case "FIND_ROOM":
			room := findOrCreateRoom()
			room.addClient(c)
			if len(room.clients) == maxClients && !room.started {
				go room.start()
			}

		case "CHAT":
			if c.room != nil && c.room.started {
				out := map[string]string{"type": "CHAT", "from": c.id, "text": in.Data}
				b, _ := json.Marshal(out)
				c.room.broadcast <- b
			}
		}
	}
}

func (c *Client) writer() {
	for {
		msg, ok := <-c.send
		if !ok {
			return
		}
		c.conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// ---- ROOM LOGIC -------------------------------------------------------------

func findOrCreateRoom() *Room {
	roomsLock.Lock()
	defer roomsLock.Unlock()
	for _, r := range rooms {
		if !r.started && len(r.clients) < maxClients {
			return r
		}
	}
	r := &Room{id: randomID("room"), clients: make(map[string]*Client), broadcast: make(chan []byte, 16)}
	rooms[r.id] = r
	go r.forwarder()
	return r
}

func (r *Room) forwarder() {
	for msg := range r.broadcast {
		r.lock.RLock()
		for _, c := range r.clients {
			select {
			case c.send <- msg:
			default:
			}
		}
		r.lock.RUnlock()
	}
}

func (r *Room) addClient(c *Client) {
	r.lock.Lock()
	r.clients[c.id] = c
	c.room = r

	joined := len(r.clients)
	r.lock.Unlock()

	out := map[string]interface{}{
		"type":  "JOIN",
		"user":  c.id,
		"count": joined,     // kaç kişi var
		"max":   maxClients, // sabit: 5
	}

	b, _ := json.Marshal(out)
	r.broadcast <- b
}

func (r *Room) removeClient(id string) {
	r.lock.Lock()
	delete(r.clients, id)
	empty := len(r.clients) == 0
	r.lock.Unlock()
	if empty {
		roomsLock.Lock()
		delete(rooms, r.id)
		roomsLock.Unlock()
	}
}

func (r *Room) start() {
	r.started = true
	r.topic = topics[rand.Intn(len(topics))]
	r.aiID = randomID("bot")
	// 1) konu bildir
	startMsg := map[string]string{"type": "TOPIC", "text": r.topic}
	b, _ := json.Marshal(startMsg)
	r.broadcast <- b

	// 2) AI’yı içeri sok küçük gecikmeyle
	time.Sleep(aiJoinDelay)
	aiJoinMsg := map[string]string{"type": "JOIN", "user": r.aiID}
	b2, _ := json.Marshal(aiJoinMsg)
	r.broadcast <- b2

	// 3) ai loop
	go r.aiLoop()

	// 4) süre dolunca tahmin ekranı
	time.AfterFunc(4*time.Minute, func() {
		r.broadcast <- []byte(`{"type":"GUESS"}`)
	})
}

func (r *Room) aiLoop() {
	ctx := context.Background()
	for {
		// bekle
		time.Sleep(15 * time.Second)

		// konversasyon geçmişini almak yerine konuya göre tek mesaj örnek
		prompt := "Konu: " + r.topic + ". Konuyu zenginleştiren 1‑2 cümlelik katkı yaz."
		req := openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{Role: "system", Content: "Sen bir kullanıcı gibi davran, kendini yapay zeka olarak belli etme."},
				{Role: "user", Content: prompt},
			},
			MaxTokens: 60,
		}

		resp, err := openaiCli.CreateChatCompletion(ctx, req)
		if err != nil || len(resp.Choices) == 0 {
			continue
		}
		text := resp.Choices[0].Message.Content
		out := map[string]string{"type": "CHAT", "from": r.aiID, "text": text}
		b, _ := json.Marshal(out)
		r.broadcast <- b
	}
}

// ---- HELPERS ----------------------------------------------------------------

func randomID(prefix string) string { return prefix + "_" + randomString(6) }
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// ---- MAIN -------------------------------------------------------------------

func main() {
	rand.Seed(time.Now().UnixNano())
	if openaiKey == "" {
		log.Fatalln("Set OPENAI_API_KEY first")
	}

	http.Handle("/", http.FileServer(http.Dir("./web")))
	http.HandleFunc("/ws", wsHandler)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
