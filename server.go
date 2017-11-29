package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"
	"os"
	"io/ioutil"
	"strings"

	cors "github.com/heppu/simple-cors"
)
// Autoload environment variables in .env
import _ "github.com/joho/godotenv/autoload"

type info struct{
	Valid bool `json:"valid"`
	CountryName string `json:"country_name"`
	Carrier	string 	`json:"carrier"`
}


var (
	// WelcomeMessage A constant to hold the welcome message
	WelcomeMessage = "Please enter the number you would like to validate and retrieve its country and carrier if any. Please make sure to add the country code."

	// sessions = {
	//   "uuid1" = Session{...},
	//   ...
	// }
	sessions = map[string]Session{}

	processor = chatbotProcess
)

type (
	// Session Holds info about a session
	//Empty interfaces are used by code that handles values of unknown type,An empty interface may hold values of any type
	Session map[string]interface{}

	// JSON Holds a JSON object
	JSON map[string]interface{}

	// Processor Alias for Process func
	Processor func(session Session, message string) (string, error)
)

func chatbotProcess(session Session, message string) (string, error) {

	message = strings.TrimSpace(message)
	if _, err := strconv.Atoi(message); err != nil {
		return fmt.Sprintf("%s is not a number!\nPlease enter a number to validate.", message),nil

	}

	url := fmt.Sprintf("http://apilayer.net/api/validate?access_key=506fc53fdab5d78d6e2ee8d52a27d984&number=%s&format=1",message)
	
	numClient := http.Client{}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	res, getErr := numClient.Do(req)
	if getErr != nil {
		log.Fatal(getErr)
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		log.Fatal(readErr)
	}
	

	info1 := info{}
	jsonErr := json.Unmarshal(body, &info1)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	index := strings.Index(info1.CountryName,"(")
	if(index!=-1){
		info1.CountryName=info1.CountryName[:index]
	}

	if info1.Carrier == ""{
		info1.Carrier = "unknown"
	}
	if info1.CountryName == ""{
		info1.CountryName = "unknown"
	}

	
	if info1.Valid {
		return fmt.Sprintf("%s is a valid number.\nThe carrier is %s.\nIts location is %s.", message,info1.Carrier,info1.CountryName),nil
	}else {
		return fmt.Sprintf("%s is not a valid number!\n Please make sure to add the country code.", message),nil
	}
}

// withLog Wraps HandlerFuncs to log requests to Stdout
func withLog(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := httptest.NewRecorder()
		fn(c, r)
		log.Printf("[%d] %-4s %s\n", c.Code, r.Method, r.URL.Path)

		for k, v := range c.HeaderMap {
			w.Header()[k] = v
		}
		w.WriteHeader(c.Code)
		c.Body.WriteTo(w)
	}
}

// writeJSON Writes the JSON equivilant for data into ResponseWriter w
func writeJSON(w http.ResponseWriter, data JSON) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ProcessFunc Sets the processor of the chatbot
func ProcessFunc(p Processor) {
	processor = p
}

// handleWelcome Handles /welcome and responds with a welcome message and a generated UUID
func handleWelcome(w http.ResponseWriter, r *http.Request) {
	// Generate a UUID.
	hasher := md5.New()
	hasher.Write([]byte(strconv.FormatInt(time.Now().Unix(), 10)))
	uuid := hex.EncodeToString(hasher.Sum(nil))

	// Create a session for this UUID
	sessions[uuid] = Session{}

	// Write a JSON containg the welcome message and the generated UUID
	writeJSON(w, JSON{
		"uuid":    uuid,
		"message": WelcomeMessage,
	})
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	// Make sure only POST requests are handled
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST requests are allowed.", http.StatusMethodNotAllowed)
		return
	}

	// Make sure a UUID exists in the Authorization header
	uuid := r.Header.Get("Authorization")
	if uuid == "" {
		http.Error(w, "Missing or empty Authorization header.", http.StatusUnauthorized)
		return
	}

	// Make sure a session exists for the extracted UUID
	session, sessionFound := sessions[uuid]
	if !sessionFound {
		http.Error(w, fmt.Sprintf("No session found for: %v.", uuid), http.StatusUnauthorized)
		return
	}

	// Parse the JSON string in the body of the request
	data := JSON{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, fmt.Sprintf("Couldn't decode JSON: %v.", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Make sure a message key is defined in the body of the request
	_, messageFound := data["message"]
	if !messageFound {
		http.Error(w, "Missing message key in body.", http.StatusBadRequest)
		return
	}

	// Process the received message
	message, err := processor(session, data["message"].(string))
	if err != nil {
		http.Error(w, err.Error(), 422 /* http.StatusUnprocessableEntity */)
		return
	}

	// Write a JSON containg the processed response
	writeJSON(w, JSON{
		"message": message,
	})
}

// handle Handles /
func handle(w http.ResponseWriter, r *http.Request) {
	body :=
		"<!DOCTYPE html><html><head><title>Chatbot</title></head><body><pre style=\"font-family: monospace;\">\n" +
			"Available Routes:\n\n" +
			"  GET  /welcome -> handleWelcome\n" +
			"  POST /chat    -> handleChat\n" +
			"  GET  /        -> handle        (current)\n" +
			"</pre></body></html>"
	w.Header().Add("Content-Type", "text/html")
	fmt.Fprintln(w, body)
}

// Engage Gives control to the chatbot
func Engage(addr string) error {
	// HandleFuncs
	mux := http.NewServeMux()
	mux.HandleFunc("/welcome", withLog(handleWelcome))
	mux.HandleFunc("/chat", withLog(handleChat))
	mux.HandleFunc("/", withLog(handle))

	// Start the server
	return http.ListenAndServe(addr, cors.CORS(mux))
}

func main() {
	// Use the PORT environment variable
	port := os.Getenv("PORT")
	// Default to 3000 if no PORT environment variable was defined
	if port == "" {
		port = "3000"
	}

	// Start the server
	fmt.Printf("Listening on port %s...\n", port)
	log.Fatalln(Engage(":" + port))
}
