package usecase

import (
	"bytes"
	"chat/domain"
	"chat/utils"
	"chat/utils/errors"
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"text/template"
	"time"
	"github.com/mailgun/mailgun-go/v4"
)

type messageUseCase struct {
	timeout           time.Duration
	mailer            utils.Mailer
	messageRepository domain.MessageRepository
	roomRepository    domain.RoomRepository
	studentRepository domain.StudentRepository
}

// NewMessageUseCase instantiates a
func NewMessageUseCase(
	t time.Duration,
	mr domain.MessageRepository,
	rr domain.RoomRepository,
	sr domain.StudentRepository,
	mailer utils.Mailer) domain.MessageUseCase {
	return &messageUseCase{timeout: t, messageRepository: mr, roomRepository: rr, studentRepository: sr, mailer: mailer}
}

func (u *messageUseCase) IsAuthorized(ctx context.Context, userID, roomID string) (authorized bool) {
	_, cancel := context.WithTimeout(context.Background(), u.timeout)
	defer cancel()

	studentChatRooms, err := u.roomRepository.GetRoomsFor(ctx, userID)
	if err != nil {
		return false
	}

	for _, room := range studentChatRooms.Rooms {
		if roomID == room.RoomID {
			authorized = true
			break
		}
	}

	return
}

func (u *messageUseCase) SaveMessage(ctx context.Context, message *domain.Message) error {
	c, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	return u.messageRepository.SaveMessage(c, message)
}

func (u *messageUseCase) EditMessage(ctx context.Context, roomID string, userID string, timeStamp time.Time, message string) (*domain.Message, error) {
	c, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	existingMessage, err := u.messageRepository.GetMessage(ctx, roomID, timeStamp)
	if err != nil {
		return nil, errors.NewNotFoundError("Message does not exist")
	}

	if userID != existingMessage.FromStudentID {
		return nil, errors.NewUnauthorizedError("Users can only edit their own messages")
	}

	if message == existingMessage.MessageBody {
		return existingMessage, nil
	}

	if message == "" {
		return nil, u.messageRepository.DeleteMessage(c, roomID, timeStamp)
	}

	existingMessage.MessageBody = message
	err = u.messageRepository.EditMessage(c, existingMessage)

	if err != nil {
		return nil, errors.NewInternalServerError(err.Error())
	}

	return existingMessage, nil
}

func (u *messageUseCase) GetMessages(ctx context.Context, roomID string, timeStamp time.Time, limit int) ([]domain.Message, error) {
	c, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	retrievedMessages, err := u.messageRepository.GetMessages(c, roomID, timeStamp, limit)

	if err != nil {
		return nil, errors.NewInternalServerError(err.Error())
	}
	return retrievedMessages, nil
}

func (u *messageUseCase) DeleteMessage(ctx context.Context, roomID string, timeStamp time.Time, userID string) error {
	c, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	existingMessage, err := u.messageRepository.GetMessage(ctx, roomID, timeStamp)
	if err != nil {
		return errors.NewNotFoundError("Message does not exist")
	}

	if userID != existingMessage.FromStudentID {
		return errors.NewUnauthorizedError("Users can only delete their own messages")
	}

	return u.messageRepository.DeleteMessage(c, roomID, timeStamp)
}

func (u *messageUseCase) JoinRequest(ctx context.Context, roomID string, userID string, timeStamp time.Time) error {
	c, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	student, err := u.studentRepository.GetStudent(c, userID)
	if err != nil {
		return errors.NewNotFoundError(fmt.Sprintf("Student does not exist: %s", err.Error()))
	}

	room, err := u.roomRepository.GetRoom(c, roomID)
	if err != nil {
		return errors.NewConflictError(fmt.Sprintf("Room with ID %s does not exist: %s", roomID, err.Error()))
	}

	for _, participant := range room.Students {
		if participant.ID == userID {
			return errors.NewConflictError(fmt.Sprintf("User %s is already in room", userID))
		}
	}

	err = u.roomRepository.UpdateParticipantPendingState(c, roomID, userID, true)
	if err != nil {
		return errors.NewInternalServerError(fmt.Sprintf("Unable to update pending state: %s", err.Error()))
	}

	m := domain.Message{
		RoomID:        roomID,
		SentTimestamp: timeStamp,
		FromStudentID: userID,
		MessageBody:   fmt.Sprintf("%s %s has requested to join your group.", student.FirstName, student.LastName)}

	return u.messageRepository.SaveMessage(c, &m)
}

func (u *messageUseCase) SendRejection(ctx context.Context, roomID string, userID string, loggedID string) error {
	c, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	room, err := u.roomRepository.GetRoom(c, roomID)
	if err != nil {
		return errors.NewNotFoundError("Room does not exist")
	}

	if loggedID != room.Admin.ID {
		return errors.NewUnauthorizedError("You are not authorized to reject because you are not admin.")
	}

	student, err := u.studentRepository.GetStudent(c, userID)
	if err != nil {
		return errors.NewNotFoundError("User does not exist")
	}

	err = u.roomRepository.RemoveParticipantFromRoom(c, userID, roomID)
	if err != nil {
		return errors.NewInternalServerError(fmt.Sprintf("Unable to remove user from room: %s", err.Error()))
	}

	_, err = createEmailBody(student, roomID)
	if err != nil {
		return errors.NewInternalServerError(fmt.Sprintf("Unable to create email: %s", err.Error()))
	}
	fmt.Println("COMPLETED EMAIL BODY!!!!")

	from:=     "soen490airbenders@gmail.com"
	user:=     "035a001030be3b"
	password:= "5a1e6e53f5f9d8"
	smtpHost:= "smtp.gmail.com"
	smtpPort:= "587"

	fmt.Println(from)
	fmt.Println(user)
	fmt.Println(password)
	fmt.Println(smtpHost)
	fmt.Println(smtpPort)

	// Your available domain names can be found here:
	// (https://app.mailgun.com/app/domains)
	var yourDomain string = "sandbox4ed61a0705f145d7a9a1f810864489f0.mailgun.org" // e.g. mg.yourcompany.com

	// You can find the Private API Key in your Account Menu, under "Settings":
	// (https://app.mailgun.com/app/account/security)
	var privateAPIKey string = "2f40f5e6fe2694aca3dff2edfd00f382-0677517f-ca0120e6"

		// Create an instance of the Mailgun Client
		mg := mailgun.NewMailgun(yourDomain, privateAPIKey)

		sender := from
		subject := "Fancy subject!"
		body := "Hello from Mailgun Go!"
		recipient := "soen390erps@gmail.com"

		// The message object allows you to add attachments and Bcc recipients
		message := mg.NewMessage(sender, subject, body, recipient)

		ctx, cancel = context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		// Send the message with a 10 second timeout
		resp, id, err := mg.Send(ctx, message)

		if err != nil {
			fmt.Println("FAILED TO SEND WITH MAILGUN")
			log.Fatal(err)
		}

		fmt.Printf("ID: %s Resp: %s\n", id, resp)

	//m := gomail.NewMessage()
	//m.SetHeader("From", from)
	//m.SetHeader("To", student.Email)
	//m.SetHeader("Subject", "Hello!")
	//m.SetBody("text/html", "Hello <b>Bob</b> and <i>Cora</i>!")

	//d := gomail.NewDialer(smtpHost, 587, "soen490airbenders@gmail.com", "airbenders-soen-490")
	//d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	//fmt.Println("NEW DIALER CREATED")
	//// Send the email to Bob, Cora and Dan.
	//if err := d.DialAndSend(m); err != nil {
	//		fmt.Println("failed gomail gmail dial and send")
	//
	//	msg := "From: " + from + "\n" +
	//		"To: " + "soen390erps@gmail.com" + "\n" +
	//		"Subject: Hello there\n\n"
	//
	//	err := smtp.SendMail("smtp.gmail.com:587",
	//		smtp.PlainAuth("", "soen490airbenders@gmail.com", "airbenders-soen-490", "smtp.gmail.com"),
	//		from, []string{"soen390erps@gmail.com"}, []byte(msg))
	//
	//	if err != nil {
	//		log.Printf("smtp error: %s", err)
	//		panic(err)
	//	}
	//
	//}

	// Create a new email - specify the SMTP host:port and auth (if needed)
	//mail := mailyak.New(fmt.Sprintf("%s:%s", smtpHost, smtpPort), nil)
	//
	//mail.To(student.Email)
	//mail.From(from)
	//mail.FromName("momas")
	//mail.Subject("REJECTION")
	//
	//// mail.HTML() and mail.Plain() implement io.Writer, so you can do handy things like
	//// parse a template directly into the email body
	//if _, err := io.WriteString(mail.HTML(), "So long, and thanks for all the fish."); err != nil {
	//	panic(" :( ")
	//}
	//
	//// Or set the body using a string setter
	////mail.Plain().Set("YOU ARE REJECTEDDD")
	//
	//// And you're done!
	//if err := mail.Send(); err != nil {
	//	panic(" UNABLE TO SEND WITH MAILYAK ")
	//}



				//conn, err := smtp.Dial(fmt.Sprintf("%s:%s", smtpHost, smtpPort))
				//fmt.Println("DIALLING IN TO SMTP ADDRESS")
				//_, err = net.Dial("tcp", fmt.Sprintf("%s:%s", smtpHost, smtpPort))
				//if err != nil {
				//	fmt.Println("COULD NOT DIAL IN TO SMTP ADDRESS")
				//	fmt.Println(err)
				//	return err
				//}
				//fmt.Println("SUCCESS DIAL IN TO SMTP ADDRESS")
					//conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", smtpHost, smtpPort), 10*time.Second)
					//if err != nil {
					//	fmt.Println("COULD NOT DIALTIMEOUT IN TO SMTP ADDRESS")
					//	fmt.Println(err)
					//}
					//fmt.Println("SUCCESS DIALTIMEOUT TO SMTP ADDRESS")

					// Connect to the SMTP server
					//c, err := smtp.NewClient(conn, smtpHost)
					//if err != nil {
					//	fmt.Println("FAILED TO CREATE NEW STMP CLIENT")
					//	fmt.Println(err)
					//}
					//defer c.Quit()
				//} else {
				//	fmt.Println("SUCCESS DIALLED INTO SMTP ADDRESS")
				//}
				//defer conn.Close()
				//
				//if err = conn.Mail(from); err != nil {
				//	return err
				//}
				//if err = conn.Rcpt(student.Email); err != nil {
				//	return err
				//}
				//w, err := conn.Data()
				//if err != nil {
				//	return err
				//}
				//msg := "To: " + strings.Join([]string{student.Email}, ",") + "\r\n" +
				//	"From: " + from + "\r\n" +
				//	"Subject: " + "TESTTT" + "\r\n" +
				//	"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
				//	"Content-Transfer-Encoding: base64\r\n" +
				//	"\r\n" + base64.StdEncoding.EncodeToString([]byte(emailBody))
				//
				//_, err = w.Write([]byte(msg))
				//if err != nil {
				//	return err
				//}
				//err = w.Close()
				//if err != nil {
				//	return err
				//}
				//return conn.Quit()

	//err = u.mailer.SendSimpleMail(student.Email, emailBody)
	//if err!= nil {
	//	fmt.Println("FAILED TO SEND MAIL")
	//	fmt.Println(err)
	//}
	return err
}

func createEmailBody(student *domain.Student, team string) ([]byte, error) {
	fmt.Println("CREATING EMAIL BODY...")
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.NewInternalServerError(fmt.Sprintf("Unable to retrieve current working directory\n %s", err))
	}

	var pathToFile string
	if strings.Contains(cwd, "bin") {
		pathToFile = path.Join(cwd, "rejection_template.html")
	} else {
		pathToFile = path.Join(cwd, "static", "rejection_template.html")
	}
	t, err := template.ParseFiles(pathToFile)
	if err != nil {
		return nil, errors.NewInternalServerError(fmt.Sprintf("Unable to find the file\n %s", err))
	}
	fmt.Println("FOUND REJECTION HTML!")

	var body bytes.Buffer
	//fmt.Println("EMAIL FROM: ")
	//fmt.Println(os.Getenv("EMAIL_FROM"))
	message := fmt.Sprintf("From: %s\r\n", "soen490airbenders@gmail.com")
	message += fmt.Sprintf("To: %s\r\n", student.Email)
	message += "Subject: Team Request\r\n"
	message += "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	message += "\r\n"

	body.Write([]byte(message))
	t.Execute(&body, struct {
		Name string
		Team string
	}{
		Name: student.FirstName,
		Team: team,
	})
	fmt.Println("COMPLETED WRITING MESSAGE TO BUFFER")
	return body.Bytes(), nil
}
