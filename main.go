package main

import (
	"database/sql"
	"encoding/json"
	"io/ioutil" // Добавим импорт пакета ioutil
	"log"
	"net/http"
	"sync"
	"time"
                 "html/template"

	"github.com/gin-gonic/gin"
	"github.com/mmcdole/gofeed"
	_ "github.com/mattn/go-sqlite3"
)

var (
	dbMutex sync.Mutex
)

type Config struct {
	Feeds   []string `json:"feeds"`
	Refresh int      `json:"refresh"`
}

type Item struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PubDate     time.Time `json:"pubDate"`
	Link        string    `json:"link"`
}

func main() {
	r := gin.Default()

                 r.LoadHTMLGlob("templates/*")

   	 r.GET("/", func(c *gin.Context) {
       	 c.String(http.StatusOK, "Welcome to the news aggregator!")
   	 })

	r.GET("/api/news/:count", GetNews)

	var config Config
	if err := LoadConfig("config.json", &config); err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite3", "news.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Создаем таблицу для хранения новостей
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS news (
		id INTEGER PRIMARY KEY,
		title TEXT,
		description TEXT,
		pub_date DATETIME,
		link TEXT
	);`)
	if err != nil {
		log.Fatal(err)
	}

	// Создаем горутину для периодического обхода RSS-лент
	go func() {
		for {
			for _, feedURL := range config.Feeds {
				go FetchAndSaveNews(db, feedURL)
			}
			time.Sleep(time.Duration(config.Refresh) * time.Minute)
		}
	}()

	r.Run(":8080")
}

func LoadConfig(filename string, config *Config) error {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(file, config)
}

func FetchAndSaveNews(db *sql.DB, feedURL string) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		log.Printf("Error parsing RSS feed: %v", err)
		return
	}

	dbMutex.Lock()
	defer dbMutex.Unlock()

	for _, item := range feed.Items {
		pubDate, _ := time.Parse(time.RFC1123Z, item.Published)
		_, err := db.Exec("INSERT INTO news (title, description, pub_date, link) VALUES (?, ?, ?, ?)",
			item.Title, item.Description, pubDate, item.Link)
		if err != nil {
			log.Printf("Error inserting news into the database: %v", err)
		}
	}
}

func GetNews(c *gin.Context) {
    count := c.Param("count")
    db, err := sql.Open("sqlite3", "news.db")
    if err != nil {
        c.String(http.StatusInternalServerError, "Internal Server Error")
        return
    }
    defer db.Close()

    rows, err := db.Query("SELECT title, description, pub_date, link FROM news ORDER BY pub_date DESC LIMIT ?", count)
    if err != nil {
        c.String(http.StatusInternalServerError, "Internal Server Error")
        return
    }
    defer rows.Close()

    var news []Item
    for rows.Next() {
        var item Item
        err := rows.Scan(&item.Title, &item.Description, &item.PubDate, &item.Link)
        if err != nil {
            c.String(http.StatusInternalServerError, "Internal Server Error")
            return
        }
        news = append(news, item)
    }

    // Генерируем HTML для вывода новостей
    html := "<h1>Latest News</h1><ul>"
    for _, item := range news {
        html += "<li><strong>" + item.Title + "</strong><br>"
        html += item.PubDate.Format("January 2, 2006 15:04:05") + "<br>"
        html += item.Description + "<br>"
        html += "<a href='" + item.Link + "' target='_blank'>Read more</a></li>"
    }
    html += "</ul>"

    c.HTML(http.StatusOK, "news.html", gin.H{
        "title": "Latest News",
        "news":  template.HTML(html),
    })
}
