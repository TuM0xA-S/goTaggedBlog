package main

import (
	"github.com/TuM0xA-S/goTaggedBlog/blog"
	"net/http"
	"log"
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const pageSize = 15

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

	blog := blog.NewBlog(posts, pageSize, "blog")
	http.Handle("/", blog)
	http.ListenAndServe("localhost:2222", nil)
}
