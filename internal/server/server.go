// Package server exposes isolated MarketLab simulations over HTTP and WebSocket.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"marketlab/internal/engine"
	"marketlab/internal/sim"
)

const (
	sessionCookie       = "marketlab_session"
	maxCommandBody      = 64 << 10
	maxImportRequest    = sim.MaxImportBytes + (1 << 20)
	maxRequestIDLength  = 128
	websocketWriteLimit = 5 * time.Second
)

type Options struct {
	MaxSessions     int
	SessionTTL      time.Duration
	CleanupInterval time.Duration
}

type Server struct {
	manager *manager
	assets  fs.FS
}

func New(assets fs.FS) *Server {
	return NewWithOptions(assets, Options{})
}

func NewWithOptions(assets fs.FS, options Options) *Server {
	if options.MaxSessions <= 0 || options.MaxSessions > defaultMaxSessions {
		options.MaxSessions = defaultMaxSessions
	}
	if options.SessionTTL <= 0 {
		options.SessionTTL = defaultSessionTTL
	}
	if options.CleanupInterval <= 0 {
		options.CleanupInterval = time.Minute
	}
	return &Server{
		manager: newManager(options.MaxSessions, options.SessionTTL, options.CleanupInterval),
		assets:  assets,
	}
}

func (s *Server) Close() {
	s.manager.close()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/session", s.getSession)
	mux.HandleFunc("POST /api/command", s.postCommand)
	mux.HandleFunc("GET /api/export", s.exportSession)
	mux.HandleFunc("POST /api/import", s.importSession)
	mux.HandleFunc("GET /ws", s.websocket)
	mux.HandleFunc("GET /", s.serveFrontend)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws: wss:; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.cookieSession(w, r, true)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	snapshot, err := session.do(r.Context(), "", false, noMutation)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

type commandEnvelope struct {
	SessionID string          `json:"sessionId"`
	RequestID string          `json:"requestId"`
	Command   json.RawMessage `json:"command"`
}

type commandPayload struct {
	Type        string              `json:"type"`
	Symbol      string              `json:"symbol,omitempty"`
	Side        string              `json:"side,omitempty"`
	OrderType   string              `json:"orderType,omitempty"`
	TimeInForce string              `json:"timeInForce,omitempty"`
	Quantity    int64               `json:"quantity,omitempty"`
	Price       *float64            `json:"price,omitempty"`
	OrderID     uint64              `json:"orderId,omitempty"`
	ScenarioID  string              `json:"scenarioId,omitempty"`
	Seed        *int64              `json:"seed,omitempty"`
	Speed       float64             `json:"speed,omitempty"`
	Elapsed     *int64              `json:"elapsed,omitempty"`
	Index       *int                `json:"index,omitempty"`
	Strategy    string              `json:"strategy,omitempty"`
	Enabled     *bool               `json:"enabled,omitempty"`
	Params      *sim.StrategyParams `json:"params,omitempty"`
}

type commandResponse struct {
	Accepted  bool          `json:"accepted"`
	RequestID string        `json:"requestId,omitempty"`
	Snapshot  *sim.Snapshot `json:"snapshot,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Message   string        `json:"message,omitempty"`
}

type serverMessage struct {
	Type       string        `json:"type"`
	Sequence   uint64        `json:"sequence,omitempty"`
	Snapshot   *sim.Snapshot `json:"snapshot,omitempty"`
	RequestID  string        `json:"requestId,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	ServerTime string        `json:"serverTime,omitempty"`
}

func (s *Server) postCommand(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCommandBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid command body: %w", err))
		return
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(body, &shape); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid command body: %w", err))
		return
	}
	var envelope commandEnvelope
	var commandBytes []byte
	var session *Session
	requestID := ""
	if _, wrapped := shape["command"]; wrapped {
		if err := decodeJSON(strings.NewReader(string(body)), &envelope); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid command envelope: %w", err))
			return
		}
		if envelope.SessionID == "" || envelope.RequestID == "" || len(envelope.RequestID) > maxRequestIDLength || len(envelope.Command) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("sessionId, requestId, and command are required"))
			return
		}
		session, err = s.ownedSession(r, envelope.SessionID)
		if err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
		commandBytes = envelope.Command
		requestID = envelope.RequestID
	} else {
		session, err = s.cookieSession(w, r, false)
		if err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
		commandBytes = body
	}
	var command commandPayload
	if err := decodeJSON(strings.NewReader(string(commandBytes)), &command); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid command: %w", err))
		return
	}
	if command.Type == "" {
		writeError(w, http.StatusBadRequest, errors.New("command type is required"))
		return
	}

	snapshot, commandErr := session.do(r.Context(), requestID, true, func(state *sim.Simulation) (*sim.Simulation, error) {
		return applyCommand(state, command)
	})
	response := commandResponse{Accepted: commandErr == nil, RequestID: requestID, Snapshot: &snapshot}
	if commandErr != nil {
		response.Reason = commandErr.Error()
		response.Message = commandErr.Error()
		writeJSON(w, http.StatusUnprocessableEntity, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func applyCommand(state *sim.Simulation, command commandPayload) (*sim.Simulation, error) {
	switch command.Type {
	case "simulation.start", "simulation.resume", "start", "resume":
		state.Running = true
	case "simulation.pause", "pause":
		state.Running = false
	case "simulation.step", "step":
		state.Running = false
		state.Step()
	case "simulation.speed", "speed":
		if command.Speed != 0.5 && command.Speed != 1 && command.Speed != 2 && command.Speed != 4 {
			return state, errors.New("speed must be 0.5, 1, 2, or 4")
		}
		state.Speed = command.Speed
	case "simulation.restart", "simulation.reset", "restart", "reset":
		seed := state.Seed
		if command.Seed != nil {
			seed = *command.Seed
		}
		replacement, err := sim.New(state.ID, seed)
		return replacement, err
	case "instrument.select":
		if command.Symbol != engine.Symbol {
			return state, fmt.Errorf("unknown instrument %q", command.Symbol)
		}
	case "order.place":
		return state, placeOrder(state, command)
	case "order.cancel":
		if command.OrderID == 0 {
			return state, errors.New("orderId must be a positive integer")
		}
		return state, state.Cancel("MANUAL", sim.HumanParticipant, command.OrderID, "Manual cancellation.")
	case "scenario.load", "scenario.trigger", "scenario":
		return state, state.TriggerScenario(command.ScenarioID)
	case "strategy.update":
		if command.Enabled == nil {
			return state, errors.New("enabled is required")
		}
		return state, state.SetStrategy(command.Strategy, *command.Enabled, command.Params)
	case "replay.enter":
		return state, state.EnterReplay()
	case "replay.exit":
		replacement, err := sim.New(state.ID, state.Seed)
		return replacement, err
	case "replay.play":
		if state.Mode != "REPLAY" {
			return state, errors.New("not in replay mode")
		}
		state.Running = true
	case "replay.pause":
		state.Running = false
	case "replay.step":
		state.Running = false
		return state, state.StepReplay()
	case "replay.seek", "simulation.seek":
		index := command.Index
		if index == nil && command.Elapsed != nil {
			converted := int(*command.Elapsed)
			index = &converted
		}
		if index == nil {
			return state, errors.New("index or elapsed is required")
		}
		return state, state.SeekReplay(*index)
	default:
		return state, fmt.Errorf("unknown command type %q", command.Type)
	}
	return state, nil
}

func placeOrder(state *sim.Simulation, command commandPayload) error {
	if command.Symbol != "" && command.Symbol != engine.Symbol {
		return fmt.Errorf("unknown instrument %q", command.Symbol)
	}
	if command.Quantity <= 0 || command.Quantity > engine.MaxQuantity {
		return fmt.Errorf("quantity must be between 1 and %d", engine.MaxQuantity)
	}
	side := engine.Side(strings.ToUpper(command.Side))
	if side != engine.Buy && side != engine.Sell {
		return errors.New("side must be buy or sell")
	}
	orderType := engine.OrderType(strings.ToUpper(command.OrderType))
	if orderType != engine.Limit && orderType != engine.Market {
		return errors.New("orderType must be market or limit")
	}
	if command.TimeInForce != "" && command.TimeInForce != "GTC" && command.TimeInForce != "IOC" && command.TimeInForce != "FOK" {
		return errors.New("timeInForce must be GTC, IOC, or FOK")
	}
	price := int64(0)
	if orderType == engine.Limit {
		if command.Price == nil || math.IsNaN(*command.Price) || math.IsInf(*command.Price, 0) || *command.Price <= 0 {
			return errors.New("a positive price is required for limit orders")
		}
		price = int64(math.Round(*command.Price))
		if price <= 0 || price > engine.MaxPrice {
			return errors.New("price is out of range")
		}
	}
	_, err := state.SubmitManual(engine.SubmitRequest{Side: side, Type: orderType, Price: price, Quantity: command.Quantity})
	return err
}

func (s *Server) exportSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	var session *Session
	var err error
	if sessionID == "" {
		session, err = s.cookieSession(w, r, false)
	} else {
		session, err = s.ownedSession(r, sessionID)
	}
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var experiment sim.Experiment
	_, err = session.do(r.Context(), "", false, func(state *sim.Simulation) (*sim.Simulation, error) {
		experiment = state.Export()
		return state, nil
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="marketlab-session.json"`)
	writeJSON(w, http.StatusOK, experiment)
}

func (s *Server) importSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.cookieSession(w, r, true)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportRequest)
	var source io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid multipart import: %w", err))
			return
		}
		file, _, err := r.FormFile("session")
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("multipart field session is required"))
			return
		}
		defer file.Close()
		source = file
	}
	imported, err := sim.Import(session.id, source)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	snapshot, err := session.do(r.Context(), "", true, func(*sim.Simulation) (*sim.Simulation, error) {
		return imported, nil
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, errors.New("websocket origin does not match host"))
		return
	}
	requestedID := r.URL.Query().Get("session")
	var session *Session
	var err error
	if requestedID == "" {
		session, err = s.cookieSession(w, r, false)
	} else {
		session, err = s.ownedSession(r, requestedID)
	}
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	connection, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(4 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	updates := make(chan []byte, 1)
	if err := session.addSubscriber(ctx, updates); err != nil {
		_ = connection.Close(websocket.StatusInternalError, "session unavailable")
		return
	}
	defer session.removeSubscriber(updates)

	go func() {
		for {
			if _, _, err := connection.Read(ctx); err != nil {
				cancel()
				return
			}
			session.touch()
		}
	}()
	for {
		select {
		case payload, ok := <-updates:
			if !ok {
				return
			}
			writeContext, cancelWrite := context.WithTimeout(ctx, websocketWriteLimit)
			err := connection.Write(writeContext, websocket.MessageText, payload)
			cancelWrite()
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) cookieSession(w http.ResponseWriter, r *http.Request, create bool) (*Session, error) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if session, ok := s.manager.get(cookie.Value); ok {
			session.touch()
			return session, nil
		}
	}
	if !create {
		return nil, errors.New("session cookie is missing or expired")
	}
	session, err := s.manager.create()
	if err != nil {
		return nil, err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: session.id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(s.manager.sessionTTL.Seconds()), Secure: r.TLS != nil,
	})
	return session, nil
}

func (s *Server) ownedSession(r *http.Request, requestedID string) (*Session, error) {
	if requestedID == "" {
		return nil, errors.New("session identifier is required")
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value != requestedID {
		return nil, errors.New("session does not belong to this visitor")
	}
	session, ok := s.manager.get(requestedID)
	if !ok {
		return nil, errors.New("session is missing or expired")
	}
	session.touch()
	return session, nil
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func (s *Server) serveFrontend(w http.ResponseWriter, r *http.Request) {
	if s.assets == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("frontend assets are unavailable"))
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	content, err := fs.ReadFile(s.assets, name)
	if err != nil {
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		content, err = fs.ReadFile(s.assets, name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(content)
}

func decodeJSON(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func noMutation(state *sim.Simulation) (*sim.Simulation, error) {
	return state, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error(), "message": err.Error()})
}

func ParsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("PORT must be between 1 and 65535")
	}
	return port, nil
}
