package app

import (
	http2 "chat/Room/delivery/http"
	roomRepository "chat/Room/repository"
	roomUseCase "chat/Room/usecase"
	"chat/messaging/delivery/http"
	"chat/messaging/repository"
	"chat/messaging/repository/cassandra"
	"chat/messaging/usecase"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"log"
	"os"
	"time"
)

func Server(mh *http.MessageHandler, rh *http2.RoomHandler, mw Middleware) *gin.Engine {
	router := gin.Default()
	mapChatUrls(mw, router, mh)
	mapRoomURLs(mw, router, rh)
	return router
}

// Start runs the server
func Start() {
	cluster := gocql.NewCluster(os.Getenv("CASSANDRA_HOST"))

	session, err := cluster.CreateSession()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Connected Cassandra database OK")


	mr := repository.NewChatRepository(cassandra.NewSession(session))
	rr := roomRepository.NewRoomRepository(cassandra.NewSession(session))
	sr := roomRepository.NewStudentRepository(cassandra.NewSession(session))

	mu := usecase.NewMessageUseCase(time.Second*2, mr, rr)
	ru := roomUseCase.NewRoomUseCase(rr, sr, time.Second*2)

	mh := http.NewMessageHandler(mu)
	rh := http2.NewRoomHandler(ru)

	su := usecase.NewStudentUseCase(*sr)

	go su.ListenStudentCreation()
	go su.ListenStudentEdit()
	go su.ListenStudentDelete()

	mw := NewMiddleware()

	mainHub := http.NewHub()
	go mainHub.StartHubListener()
	router := Server(mh, rh, mw)
	router.Run()
}
