package main

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const pageSize = 7

type Post struct {
	TimePublished time.Time     `bson:"timePublished"`
	Title         string        `bson:"title"`
	Tags          []string      `bson:"tags"`
	Body          template.HTML `bson:"body"`
	ID            int           `bson:"_id"`
}

type blogPageData struct {
	Title      string
	PageNumber int
	Posts      []Post
	PageCount  int
}

var blogPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
	"dec": func(x int) int {
		return x - 1
	},
	"inc": func(x int) int {
		return x + 1
	},
}).ParseFiles(
	"templates/base.html",
	"templates/post-header.html",
	"templates/post-list.html",
))

var postPage = template.Must(template.New("base.html").ParseFiles(
	"templates/base.html",
	"templates/post-header.html",
	"templates/post.html",
))

func main() {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:33017"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	posts := client.Database("test").Collection("posts")

	http.HandleFunc("/static/style.css", func(rw http.ResponseWriter, r *http.Request) {
		http.ServeFile(rw, r, "static/style.css")
	})

	http.HandleFunc("/blog/page/", func(rw http.ResponseWriter, r *http.Request) {
		var pageNumber int
		if r.URL.Path == "/blog/page/" {
			pageNumber = 1
		} else {
			path := strings.Split(r.URL.Path, "/")
			var err error
			pageNumber, err = strconv.Atoi(path[len(path)-1])
			if err != nil {
				http.NotFound(rw, r)
				return
			}
		}
		pg := &blogPageData{Title: "Tagged Blog", Posts: []Post{}, PageNumber: pageNumber}
		cursor, err := posts.Find(context.TODO(), bson.M{})
		if err != nil {
			log.Fatal(err)
		}
		lowest := 1 + pageSize*(pageNumber-1)
		biggest := pageSize * pageNumber
		cnt := 0
		for cursor.Next(context.TODO()) {
			cnt++
			if cnt >= lowest && cnt <= biggest {
				var post Post
				if err := cursor.Decode(&post); err != nil {
					log.Fatal(err)
				}
				pg.Posts = append(pg.Posts, post)
			}
		}

		pg.PageCount = (cnt + pageSize - 1) / pageSize
		if err := blogPage.Execute(rw, pg); err != nil {
			log.Fatal(err)
		}
	})

	http.HandleFunc("/blog/post/", func(rw http.ResponseWriter, r *http.Request) {
		path := strings.Split(r.URL.Path, "/")
		id, err := strconv.Atoi(path[len(path)-1])
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		var post Post
		if err := posts.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&post); err != nil {
			http.NotFound(rw, r)
			return
		}
		err = postPage.Execute(rw, post)
		if err != nil {
			log.Fatal(err)
		}
	})

	http.Handle("/blog/", http.RedirectHandler("/blog/page/1", http.StatusFound))

	http.HandleFunc("/blog/admin", func(rw http.ResponseWriter, r *http.Request){

	})

	http.HandleFunc("/blog/admin/auth", func(rw http.ResponseWriter, r *http.Request){
		
	})

	http.ListenAndServe("localhost:2222", nil)
}
