package main

import (
	"context"
	"log"
	"net/http"

	"github.com/TuM0xA-S/goTaggedBlog/blog"
	"github.com/caarlos0/env"
	"github.com/urfave/negroni"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	MongodbURI string `env:"MONDODB_URI"`
	PageSize   int    `env:"PAGE_SIZE"`
	Host       string `env:"HOST"`
	Login      string `env:"LOGIN"`
	Password   string `env:"PASSWORD"`
	SecretKey  string `env:"SECRET_KEY"`
	BlogTitle  string `env:"BLOG_TITLE"`
}

func main() {
	cfg := &config{}
	err := env.Parse(cfg)
	if err != nil {
		log.Fatal("when parsing env:", err)
	}
	ctx := context.Background()
	log.Println("connecting to db at " + cfg.MongodbURI)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongodbURI))
	if err != nil {
		log.Fatal("when connecting to db:", err)
	}
	log.Println("connected!!!")
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	posts := client.Database("blog").Collection("posts")

	blog := blog.NewBlog(posts, cfg.PageSize, "blog", cfg.Login, cfg.Password, cfg.SecretKey, cfg.BlogTitle)

	n := negroni.New(negroni.NewLogger(), negroni.NewRecovery())
	n.UseHandler(blog)

	http.Handle("/", n)
	if err := http.ListenAndServe(cfg.Host, nil); err != nil {
		log.Fatal("when serving:", err)
	}
}
