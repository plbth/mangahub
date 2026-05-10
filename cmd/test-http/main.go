package main

import (
	"flag"
	"log"

	httpapi "github.com/plbth/mangahub/internal/http"
)

func main() {
	var (
		addr      = flag.String("addr", ":8080", "HTTP listen address")
		dataPath  = flag.String("data", "data/manga.json", "path to manga JSON seed file")
		jwtSecret = flag.String("jwt-secret", "mangahub-dev-secret", "JWT signing secret")
	)
	flag.Parse()

	srv, err := httpapi.NewServer(httpapi.Config{
		DataPath:  *dataPath,
		JWTSecret: *jwtSecret,
	})
	if err != nil {
		log.Fatalf("create HTTP server: %v", err)
	}

	log.Printf("MangaHub HTTP API listening on %s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("run HTTP server: %v", err)
	}
}
