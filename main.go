package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/TuM0xA-S/goTaggedBlog/blog"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const pageSize = 8

type config struct {
	MongodbURI     string
	PageSize       int
	Host           string
	Login          string
	Password       string
	SecretKey      string
	DBName         string
	CollectionName string
	BlogTitle      string
}

func main() {
	cfgFile, err := os.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}
	decoder := json.NewDecoder(cfgFile)
	var cfg config
	decoder.Decode(&cfg)
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongodbURI))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	posts := client.Database(cfg.DBName).Collection(cfg.CollectionName)

	blog := blog.NewBlog(posts, pageSize, "blog", cfg.Login, cfg.Password, cfg.SecretKey, cfg.BlogTitle)
	http.Handle("/", blog)
	http.ListenAndServe(cfg.Host, nil)
}
