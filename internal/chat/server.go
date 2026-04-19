package chat

import (
	"botDashboard/internal/event"
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/gobwas/ws"

	"botDashboard/pkg/broker"
)

type ServerOption func(*Server)

type Server struct {
	Hub         *Hub
	publisher   CommandPublisher
	disableNATS bool
	mu          sync.Mutex
	listeners   []listenerStopper
}

type listenerStopper interface {
	Stop()
}

func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		Hub:       NewHub(),
		publisher: clientPublisher{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithHub(h *Hub) ServerOption {
	return func(s *Server) {
		if h != nil {
			s.Hub = h
		}
	}
}

func WithCommandPublisher(p CommandPublisher) ServerOption {
	return func(s *Server) {
		if p != nil {
			s.publisher = p
		}
	}
}

func WithNATSDisabledForTest() ServerOption {
	return func(s *Server) {
		s.disableNATS = true
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/chat/ws" {
		http.NotFound(w, r)
		return
	}

	user, err := s.authenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}

	client := newClient(conn, s.Hub, user, s.publisher)
	s.Hub.Register(client)
	go client.writeLoop()
	go client.readLoop()
}

func (s *Server) authenticate(r *http.Request) (model.UserData, error) {
	tokenStr, err := authTokenFromRequest(r)
	if err != nil {
		return model.UserData{}, err
	}

	claims, err := middleware.GetJwt().ValidateToken(tokenStr)
	if err != nil {
		return model.UserData{}, err
	}

	login := claims.Login
	if login == "" {
		login = claims.Email
	}
	return model.UserData{Email: claims.Email, Login: login}, nil
}

func authTokenFromRequest(r *http.Request) (string, error) {
	if auth := r.Header.Get("Authorization"); auth != "" {
		return middleware.ParseBearerToken(auth)
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		return "", errors.New("authorization required")
	}
	return token, nil
}

func (s *Server) Start(ctx context.Context, addr string) error {
	if !s.disableNATS {
		if os.Getenv("NATS_URL") == "" {
			return fmt.Errorf("NATS_URL is required for chat server")
		}
		if err := s.startListeners(ctx); err != nil {
			return err
		}
	}

	srv := &http.Server{Addr: addr, Handler: s}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) startListeners(ctx context.Context) error {
	b := broker.Get()
	go func() {
		<-ctx.Done()
		b.Close()
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners,
		broker.NewListener[event.ChatMessagePersistedEvent](ctx, b.Nc, event.ChatEventMessagePersisted, s.Hub.HandleChatMessagePersisted),
		broker.NewListener[event.ChatMessageUpdatedEvent](ctx, b.Nc, event.ChatEventMessageUpdated, s.Hub.HandleChatMessageUpdated),
		broker.NewListener[event.ChatMessageDeletedEvent](ctx, b.Nc, event.ChatEventMessageDeleted, s.Hub.HandleChatMessageDeleted),
		broker.NewListener[event.ChatMessageReadUpdatedEvent](ctx, b.Nc, event.ChatEventMessageReadUpdated, s.Hub.HandleChatMessageReadUpdated),
		broker.NewListener[event.ChatConversationUpdatedEvent](ctx, b.Nc, event.ChatEventConversationUpdated, s.Hub.HandleChatConversationUpdated),
	)
	return nil
}
