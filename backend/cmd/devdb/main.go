// devdb: chạy PostgreSQL nhúng cho máy dev (không cần Docker, không cần cài Postgres).
//
//	go run ./cmd/devdb                 # postgres://postgres:postgres@localhost:5432/vuathau, dữ liệu ở ./.devdb
//	go run ./cmd/devdb -port 5433
//
// Ctrl+C để dừng. Lần đầu tự tải bản Postgres (vài chục MB). Chỉ dùng khi dev, chạy thật dùng Postgres thường.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	port := flag.Uint("port", 5432, "cổng Postgres")
	dir := flag.String("dir", ".devdb", "thư mục dữ liệu")
	flag.Parse()
	abs, _ := filepath.Abs(*dir)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(uint32(*port)).Username("postgres").Password("postgres").Database("vuathau").
		Encoding("UTF8").Locale("C"). // tiếng Việt: bắt buộc UTF-8 (Windows mặc định WIN1252)
		DataPath(filepath.Join(abs, "data")).RuntimePath(filepath.Join(abs, "run")).
		BinariesPath(filepath.Join(abs, "bin")))
	if err := pg.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "không chạy được Postgres:", err)
		os.Exit(1)
	}
	fmt.Printf("Postgres đang chạy: postgres://postgres:postgres@localhost:%d/vuathau?sslmode=disable (Ctrl+C để dừng)\n", *port)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	_ = pg.Stop()
}
