package main

import (
	"fmt"
	"os"
	"text/template"
)

func main() {
	a := struct {
		Posts      []string
		Page       int
		TotalPages int
	}{
		Posts:      []string{"first post", "second post", "third post"},
		Page:       2,
		TotalPages: 10,
	}

	tmpl := template.Must(template.ParseFiles("root.txt", "overlay.txt"))
	fmt.Println(tmpl.Name())

	err := tmpl.Execute(os.Stdout, a)
	if err != nil {
		panic(err)
	}

}
