package main

import (
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
)

func main() {

	//load .env file from given path
	//godotenv.Load(".env")

	//app.Start()

	from := "john.doe@example.com"
	user := "035a001030be3b"
	password := "5a1e6e53f5f9d8"

	to := []string{
		"roger.roe@example.com",
	}

	addr := "smtp.mailtrap.io:2525"
	host := "smtp.mailtrap.io"

	msg := []byte("From: john.doe@example.com\r\n" +
		"To: roger.roe@example.com\r\n" +
		"Subject: Test mail\r\n\r\n" +
		"Email body\r\n")
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Hostname: %s\n", hostname)

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	fmt.Printf("HOST IP?: %s\n", localAddr)

	auth := smtp.PlainAuth("", user, password, host)
fmt.Println("PLAIN AUTH USED")
	err = smtp.SendMail(addr, auth, from, to, msg)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Email sent successfully")
}
