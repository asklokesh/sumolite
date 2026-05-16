package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asklokesh/sumolite/internal/capture"
	"github.com/asklokesh/sumolite/internal/session"
	"github.com/asklokesh/sumolite/internal/signaling"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
	case "pair":
		fmt.Println("Pairing is interactive — run `sumolite serve` and follow the printed URL.")
	case "update":
		runUpdate()
	case "version":
		fmt.Println("sumolite", version)
	case "doctor":
		runDoctor()
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `sumolite — ultra-low-latency remote desktop

usage:
  sumolite serve     start the server on this machine
  sumolite update    self-update to the latest release
  sumolite doctor    check that capture + encode + input are available
  sumolite version   print version`)
	os.Exit(2)
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":7878", "listen address for signaling / web client")
	bitrate := fs.Int("bitrate", 25_000, "target bitrate in kbps")
	encoder := fs.String("encoder", "auto", "encoder: auto | h264 | h265 | av1")
	testSrc := fs.Bool("test-source", false, "use synthetic video pattern instead of screen capture (for smoke testing)")
	fs.Parse(args)

	token := os.Getenv("SUMOLITE_TOKEN")
	if token == "" {
		token = randCode(6)
	}

	cap, err := capture.Detect()
	if err != nil {
		log.Fatalf("capture detect: %v", err)
	}
	if *testSrc {
		cap.UseTestSource()
		cap.Name = "test source (videotestsrc)"
	}
	log.Printf("capture backend: %s  encoder: %s", cap.Name, cap.Encoder(*encoder))

	hub := session.NewFromBackend(cap, *bitrate, *encoder, 60)
	mux := http.NewServeMux()
	mux.Handle("/ws", signaling.NewHandler(hub, token))
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	printPairing(*addr, token)
	log.Printf("listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func printPairing(addr, token string) {
	host := firstNonLoopback()
	port := addr
	if port[0] == ':' {
		port = port[1:]
	}
	fmt.Println()
	fmt.Println("  sumolite is ready.")
	fmt.Println()
	fmt.Printf("  Open:   http://%s:%s\n", host, port)
	fmt.Printf("  Code:   %s\n", token)
	fmt.Println()
}

func firstNonLoopback() string {
	ifs, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "localhost"
}

func randCode(n int) string {
	b := make([]byte, n/2+1)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
