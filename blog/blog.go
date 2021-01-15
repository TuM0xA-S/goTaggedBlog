package blog

import (
	"net/http"
	"time"
	"context"
	"html/template"
	"log"
	"strconv"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

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



type Blog struct {
	*mux.Router
	posts *mongo.Collection
	pageSize int
	postPage *template.Template
	blogPage *template.Template
}

func (b *Blog) postListHandler(rw http.ResponseWriter, r *http.Request) {
	pageNumber, _ := strconv.Atoi(mux.Vars(r)["page"])
	pg := &blogPageData{Title: "Tagged Blog", Posts: []Post{}, PageNumber: pageNumber}
	cursor, err := b.posts.Find(context.TODO(), bson.M{})
	if err != nil {
		log.Fatal(err)
	}
	lowest := 1 + b.pageSize*(pageNumber-1)
	biggest := b.pageSize * pageNumber
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

	pg.PageCount = (cnt + b.pageSize - 1) / b.pageSize
	if err := b.blogPage.Execute(rw, pg); err != nil {
		log.Fatal(err)
	}
}

func (b *Blog) postHandler(rw http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	var post Post
	if err := b.posts.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&post); err != nil {
		http.NotFound(rw, r)
		return
	}
	
	if err := b.postPage.Execute(rw, post); err != nil {
		log.Fatal(err)
	}
}

//NewBlog creates blog app
func NewBlog(posts *mongo.Collection, pageSize int, workdir string) Blog {
	var blog Blog

	blog.blogPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"dec": func(x int) int {
			return x - 1
		},
		"inc": func(x int) int {
			return x + 1
		},
	}).ParseFiles(
		workdir + "/templates/base.html",
		workdir + "/templates/post-header.html",
		workdir + "/templates/post-list.html",
	))
	
	blog.postPage = template.Must(template.New("base.html").ParseFiles(
		workdir + "/templates/base.html",
		workdir + "/templates/post-header.html",
		workdir + "/templates/post.html",
	))
	
	blog.Router = mux.NewRouter()
	blog.posts = posts
	blog.pageSize = pageSize
	blog.HandleFunc("/blog/page/{page:[0-9]+}", blog.postListHandler)
	blog.HandleFunc("/blog/post/{id:[0-9]+}", blog.postHandler)
	blog.HandleFunc("/blog/static/style.css", func(rw http.ResponseWriter, r *http.Request) {
		http.ServeFile(rw, r, workdir + "/static/style.css")
	})
	return blog
}
