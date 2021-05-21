package blog

import (
	"context"
	"html/template"
	"net/http"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	baseDir     string
	templateDir string
	staticDir   string
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	baseDir = path.Dir(filename)
	templateDir = path.Join(baseDir, "templates")
	staticDir = path.Join(baseDir, "static")
}

type templates struct {
	postPage       *template.Template
	authPage       *template.Template
	blogPage       *template.Template
	adminPage      *template.Template
	postChangePage *template.Template
	postCreatePage *template.Template
}

func (b *Blog) parseTemplates() {
	base := func() *template.Template {
		return template.New("base.tmpl").Funcs(template.FuncMap{
			"dec": func(x int) int {
				return x - 1
			},
			"inc": func(x int) int {
				return x + 1
			},
			"join":   strings.Join,
			"getURL": b.getURL,
			"blogTitle": func() string {
				return b.title
			},
		})
	}

	b.templates.blogPage = template.Must(base().ParseFiles(
		templateDir+"/base.tmpl",
		templateDir+"/post-header.tmpl",
		templateDir+"/post-list.tmpl",
	))

	b.templates.postPage = template.Must(base().ParseFiles(
		templateDir+"/base.tmpl",
		templateDir+"/post-header.tmpl",
		templateDir+"/post.tmpl",
	))

	b.templates.authPage = template.Must(base().ParseFiles(
		templateDir+"/base.tmpl",
		templateDir+"/auth.tmpl",
	))

	b.templates.adminPage = template.Must(base().ParseFiles(
		templateDir+"/base.tmpl",
		templateDir+"/admin.tmpl",
	))

	b.templates.postCreatePage = template.Must(base().ParseFiles(
		templateDir+"/base.tmpl",
		templateDir+"/post-form.tmpl",
		templateDir+"/post-create-form.tmpl",
	))

	b.templates.postChangePage = template.Must(base().ParseFiles(
		templateDir+"/base.tmpl",
		templateDir+"/post-form.tmpl",
		templateDir+"/post-change-form.tmpl",
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

type blogPageData struct {
	Title      string
	PageNumber int
	Posts      []Post
	PageCount  int
	Query      template.URL
	Tags       []string
}

type postFormPageData struct {
	Form map[string]string
}

// Blog app
type Blog struct {
	*mux.Router
	posts                             *mongo.Collection
	counter                           *mongo.Collection
	pageSize                          int
	login, password, secretKey, title string
	templates                         templates
}

func (b *Blog) postListHandler(rw http.ResponseWriter, r *http.Request) {
	pageNumber, _ := strconv.Atoi(mux.Vars(r)["page"])

	tags := extractTags(r.URL.Query().Get("tags"))

	pg := &blogPageData{Title: b.title, Posts: []Post{}, PageNumber: pageNumber, Query: template.URL(r.URL.RawQuery), Tags: tags}

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
	if err := b.templates.blogPage.Execute(rw, pg); err != nil {
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

	if err := b.templates.postPage.Execute(rw, post); err != nil {
		panic(err)
	}
}

func (b *Blog) adminHandler(rw http.ResponseWriter, r *http.Request) {
	if !b.checkAuth(r) {
		http.Redirect(rw, r, b.getURL("auth"), http.StatusFound)
	} else if err := b.templates.adminPage.Execute(rw, nil); err != nil {
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
			http.Redirect(rw, r, b.getURL("admin"), http.StatusFound)
		} else {
			http.Redirect(rw, r, b.getURL("auth"), http.StatusFound)
		}
	} else if b.checkAuth(r) {
		http.Redirect(rw, r, b.getURL("admin"), http.StatusFound)
	} else if err := b.templates.authPage.Execute(rw, struct{ Title, BlogTitle string }{Title: "Authorization", BlogTitle: b.title}); err != nil {
		panic(err)
	}
}

// bad way, should use mongo increment instead
var mu sync.Mutex

func (b *Blog) nextID() int {
	var cnt struct {
		ID    int `bson:"_id"`
		Count int
	}
	mu.Lock()
	defer mu.Unlock()
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
		http.Redirect(rw, r, b.getURL("auth"), http.StatusFound)
		return
	}
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Redirect(rw, r, b.getURL("createPost"), http.StatusFound)
		} else {
			b.posts.InsertOne(context.TODO(), Post{
				Title:         r.Form.Get("title"),
				Body:          template.HTML(r.Form.Get("body")),
				Tags:          extractTags(r.Form.Get("tags")),
				ID:            b.nextID(),
				TimePublished: time.Now(),
			})
			http.Redirect(rw, r, b.getURL("admin"), http.StatusFound)
		}
	} else if err := b.templates.postCreatePage.Execute(rw, nil); err != nil {
		panic(err)
	}
}

func (b *Blog) changeHandler(rw http.ResponseWriter, r *http.Request) {
	if !b.checkAuth(r) {
		http.Redirect(rw, r, b.getURL("auth"), http.StatusFound)
		return
	}

	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Redirect(rw, r, b.getURL("admin"), http.StatusFound)
		} else {
			b.posts.UpdateOne(context.TODO(), bson.M{"_id": id}, bson.M{
				"$set": bson.M{
					"title":         r.Form.Get("title"),
					"body":          template.HTML(r.Form.Get("body")),
					"tags":          extractTags(r.Form.Get("tags")),
					"timePublished": time.Now(),
				}})
			http.Redirect(rw, r, b.getURL("admin"), http.StatusFound)
		}
	} else {
		var post Post
		if err := b.posts.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&post); err != nil {
			http.NotFound(rw, r)
			return
		}
		if err := b.templates.postChangePage.Execute(rw, postFormPageData{Form: map[string]string{
			"Body":  string(post.Body),
			"Title": post.Title,
			"Tags":  strings.Join(post.Tags, " "),
		}}); err != nil {
			panic(err)
		}
	}

}

func (b *Blog) removeHandler(rw http.ResponseWriter, r *http.Request) {
	if !b.checkAuth(r) {
		http.Redirect(rw, r, b.getURL("auth"), http.StatusFound)
		return
	}

	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	b.posts.FindOneAndDelete(context.TODO(), bson.M{"_id": id})
	http.Redirect(rw, r, b.getURL("admin"), http.StatusFound)
}

func (b *Blog) checkAuth(r *http.Request) bool {
	key, err := r.Cookie("auth")
	return err == nil && key.Value == b.secretKey
}

func (b *Blog) getURL(name string, pairs ...string) string {
	path, err := b.Get(name).URL(pairs...)
	if err != nil {
		panic(err)
	}

	return path.String()
}

//NewBlog creates blog app
func NewBlog(baseRouter *mux.Router, posts *mongo.Collection, pageSize int, login, password, secretKey, title string) *Blog {
	blog := &Blog{}
	blog.login = login
	blog.password = password
	blog.secretKey = secretKey
	blog.title = title

	blog.Router = baseRouter
	blog.posts = posts
	blog.counter = posts.Database().Collection(posts.Name() + ".counter")
	blog.pageSize = pageSize

	blog.parseTemplates()

	blog.HandleFunc("/page/{page:[0-9]+}", blog.postListHandler).Name("postList")
	blog.HandleFunc("/post/{id:[0-9]+}", blog.postHandler).Name("post")
	blog.HandleFunc("/static/style.css", func(rw http.ResponseWriter, r *http.Request) {
		http.ServeFile(rw, r, staticDir+"/style.css")
	})
	blog.HandleFunc("/admin/auth", blog.authHandler).Name("auth")
	blog.HandleFunc("/admin", blog.adminHandler).Name("admin")
	blog.HandleFunc("/admin/create", blog.createHandler).Name("createPost")
	blog.HandleFunc("/admin/change/{id:[0-9]+}", blog.changeHandler).Name("changePost")
	blog.HandleFunc("/admin/remove/{id:[0-9]+}", blog.removeHandler).Name("removePost")
	blog.Handle("/", http.RedirectHandler(blog.getURL("postList", "page", "1"), http.StatusFound))

	return blog
}
