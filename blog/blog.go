package blog

import (
	"context"
	"html/template"
	"net/http"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	baseDir      string
	templateDir  string
	postPage     *template.Template
	authPage     *template.Template
	blogPage     *template.Template
	adminPage    *template.Template
	postFormPage *template.Template
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	baseDir = path.Dir(filename)
	templateDir = path.Join(baseDir, "templates")
	blogPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"dec": func(x int) int {
			return x - 1
		},
		"inc": func(x int) int {
			return x + 1
		},
		"join": strings.Join,
	}).ParseFiles(
		templateDir+"/base.html",
		templateDir+"/post-header.html",
		templateDir+"/post-list.html",
	))

	postPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"join": strings.Join,
	}).ParseFiles(
		templateDir+"/base.html",
		templateDir+"/post-header.html",
		templateDir+"/post.html",
	))

	authPage = template.Must(template.New("base.html").ParseFiles(
		templateDir+"/base.html",
		templateDir+"/auth.html",
	))

	adminPage = template.Must(template.New("base.html").ParseFiles(
		templateDir+"/base.html",
		templateDir+"/admin.html",
	))

	postFormPage = template.Must(template.New("base.html").ParseFiles(
		templateDir+"/base.html",
		templateDir+"/post-form.html",
	))

}

// Post struct
type Post struct {
	TimePublished time.Time     `bson:"timePublished"`
	Title         string        `bson:"title"`
	Tags          []string      `bson:"tags"`
	Body          template.HTML `bson:"body"`
	ID            int           `bson:"_id"`
}

type postPageData struct {
	Post
	BlogTitle string
}

type blogPageData struct {
	Title      string
	PageNumber int
	Posts      []Post
	PageCount  int
	Query      template.URL
	Tags       []string
	BlogTitle  string
}

type postFormPageData struct {
	Title      string
	Form       map[string]string
	ButtonText string
	BlogTitle  string
}

// Blog app
type Blog struct {
	*mux.Router
	posts                             *mongo.Collection
	counter                           *mongo.Collection
	pageSize                          int
	login, password, secretKey, title string
}

func (b *Blog) postListHandler(rw http.ResponseWriter, r *http.Request) {
	pageNumber, _ := strconv.Atoi(mux.Vars(r)["page"])

	tags := extractTags(r.URL.Query().Get("tags"))

	pg := &blogPageData{Title: b.title, BlogTitle: b.title, Posts: []Post{}, PageNumber: pageNumber, Query: template.URL(r.URL.RawQuery), Tags: tags}

	query := bson.A{
		bson.M{
			"$match": bson.M{
				"tags": bson.M{"$in": tags},
			},
		},
		bson.M{
			"$addFields": bson.M{
				"commonCnt": bson.M{
					"$size": bson.M{
						"$setIntersection": bson.A{"$tags", tags},
					},
				},
			},
		},
		bson.M{
			"$sort": bson.D{
				{"commonCnt", -1},
				{"timePublished", -1},
			},
		},
		bson.M{
			"$skip": b.pageSize * (pageNumber - 1),
		},
		bson.M{
			"$limit": b.pageSize,
		},
	}

	if len(tags) == 0 {
		query = query[1:]
	}

	cursor, err := b.posts.Aggregate(context.TODO(), query)
	if err != nil {
		panic(err)
	}

	if err := cursor.All(context.TODO(), &pg.Posts); err != nil {
		panic(err)
	}

	criteria := bson.M{}
	if len(tags) > 0 {
		criteria["tags"] = bson.M{
			"$in": tags,
		}
	}

	cnt, err := b.posts.CountDocuments(context.TODO(), criteria)

	pg.PageCount = (int(cnt) + b.pageSize - 1) / b.pageSize
	if err := blogPage.Execute(rw, pg); err != nil {
		panic(err)
	}
}

func (b *Blog) postHandler(rw http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	var post Post
	if err := b.posts.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&post); err != nil {
		http.NotFound(rw, r)
		return
	}

	if err := postPage.Execute(rw, postPageData{post, b.title}); err != nil {
		panic(err)
	}
}

func (b *Blog) adminHandler(rw http.ResponseWriter, r *http.Request) {
	if !b.checkAuth(r) {
		http.Redirect(rw, r, "/blog/admin/auth", http.StatusFound)
	} else if err := adminPage.Execute(rw, postFormPageData{BlogTitle: b.title, Title: "ADMIN PAGE"}); err != nil {
		panic(err)
	}
}

func (b *Blog) authHandler(rw http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		r.ParseForm()
		login := r.Form.Get("login")
		password := r.Form.Get("password")
		if login == b.login && password == b.password {
			http.SetCookie(rw, &http.Cookie{Name: "auth", Value: b.secretKey})
			http.Redirect(rw, r, "/blog/admin", http.StatusFound)
		} else {
			http.Redirect(rw, r, "/blog/admin/auth", http.StatusFound)
		}
	} else if b.checkAuth(r) {
		http.Redirect(rw, r, "/blog/admin", http.StatusFound)
	} else if err := authPage.Execute(rw, struct{ Title, BlogTitle string }{Title: "Authorization", BlogTitle: b.title}); err != nil {
		panic(err)
	}
}

func (b *Blog) nextID() int {
	var cnt struct {
		ID    int `bson:"_id"`
		Count int
	}

	b.counter.FindOne(context.TODO(), bson.M{"_id": 0}).Decode(&cnt)
	cnt.Count++
	b.counter.ReplaceOne(context.TODO(), bson.M{"_id": 0}, cnt, options.Replace().SetUpsert(true))
	return cnt.Count
}

func extractTags(s string) []string {
	fields := strings.Fields(s)
	set := map[string]bool{}
	res := []string{}
	for _, f := range fields {
		f = strings.ToLower(f)
		if set[f] {
			continue
		}
		res = append(res, f)
		set[f] = true
	}

	return res
}

func (b *Blog) createHandler(rw http.ResponseWriter, r *http.Request) {
	if !b.checkAuth(r) {
		http.Redirect(rw, r, "/blog/admin/auth", http.StatusFound)
		return
	}
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Redirect(rw, r, "/blog/admin/create", http.StatusFound)
		} else {
			b.posts.InsertOne(context.TODO(), Post{
				Title:         r.Form.Get("title"),
				Body:          template.HTML(r.Form.Get("body")),
				Tags:          extractTags(r.Form.Get("tags")),
				ID:            b.nextID(),
				TimePublished: time.Now(),
			})
			http.Redirect(rw, r, "/blog/admin", http.StatusFound)
		}
	} else if err := postFormPage.Execute(rw, postFormPageData{ButtonText: "CREATE", Title: "CREATE NEW POST", BlogTitle: b.title}); err != nil {
		panic(err)
	}
}

func (b *Blog) changeHandler(rw http.ResponseWriter, r *http.Request) {
	if !b.checkAuth(r) {
		http.Redirect(rw, r, "/blog/admin/auth", http.StatusFound)
		return
	}

	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Redirect(rw, r, "/blog/admin", http.StatusFound)
		} else {
			b.posts.UpdateOne(context.TODO(), bson.M{"_id": id}, bson.M{
				"$set": bson.M{
					"title":         r.Form.Get("title"),
					"body":          template.HTML(r.Form.Get("body")),
					"tags":          extractTags(r.Form.Get("tags")),
					"timePublished": time.Now(),
				}})
			http.Redirect(rw, r, "/blog/admin", http.StatusFound)
		}
	} else {
		var post Post
		if err := b.posts.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&post); err != nil {
			http.NotFound(rw, r)
			return
		}
		if err := postFormPage.Execute(rw, postFormPageData{ButtonText: "CHANGE", Title: "CHANGE POST", Form: map[string]string{
			"Body":  string(post.Body),
			"Title": post.Title,
			"Tags":  strings.Join(post.Tags, " "),
		}, BlogTitle: b.title}); err != nil {
			panic(err)
		}
	}

}

func (b *Blog) removeHandler(rw http.ResponseWriter, r *http.Request) {
	if !b.checkAuth(r) {
		http.Redirect(rw, r, "/blog/admin/auth", http.StatusFound)
		return
	}

	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	b.posts.FindOneAndDelete(context.TODO(), bson.M{"_id": id})
	http.Redirect(rw, r, "/blog/admin", http.StatusFound)
}

func (b *Blog) checkAuth(r *http.Request) bool {
	key, err := r.Cookie("auth")
	return err == nil && key.Value == b.secretKey
}

//NewBlog creates blog app
func NewBlog(posts *mongo.Collection, pageSize int, login, password, secretKey, title string) Blog {
	var blog Blog
	blog.login = login
	blog.password = password
	blog.secretKey = secretKey
	blog.title = title

	blog.Router = mux.NewRouter()
	blog.posts = posts
	blog.counter = posts.Database().Collection(posts.Name() + ".counter")
	blog.pageSize = pageSize
	blog.HandleFunc("/blog/page/{page:[0-9]+}", blog.postListHandler)
	blog.HandleFunc("/blog/post/{id:[0-9]+}", blog.postHandler)
	blog.HandleFunc("/blog/static/style.css", func(rw http.ResponseWriter, r *http.Request) {
		http.ServeFile(rw, r, baseDir+"/static/style.css")
	})
	blog.HandleFunc("/blog/admin/auth", blog.authHandler)
	blog.HandleFunc("/blog/admin", blog.adminHandler)
	blog.HandleFunc("/blog/admin/create", blog.createHandler)
	blog.HandleFunc("/blog/admin/change/{id:[0-9]+}", blog.changeHandler)
	blog.HandleFunc("/blog/admin/remove/{id:[0-9]+}", blog.removeHandler)
	blog.Handle("/blog/", http.RedirectHandler("/blog/page/1", http.StatusFound))

	return blog
}
