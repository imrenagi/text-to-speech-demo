package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"cloud.google.com/go/storage"
	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"github.com/google/uuid"
	texttospeechpb "google.golang.org/genproto/googleapis/cloud/texttospeech/v1"
)

func main() {
	// Instantiates a client.
	ctx := context.Background()

	textToSpeechClient, err := texttospeech.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer textToSpeechClient.Close()

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("storage.NewClient: %v", err)
	}
	defer gcsClient.Close()

	http.HandleFunc("/synthesize", synthesizeText(textToSpeechClient, gcsClient))
	http.HandleFunc("/", hc())
	http.ListenAndServe(":8080", nil)
}

func hc() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("ok"))
	}
}

func GenerateName() string {
	id := uuid.New()
	return fmt.Sprintf("%s.mp3", id.String())
}

func synthesizeText(ttsClient *texttospeech.Client, gcsClient *storage.Client) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		keys, ok := request.URL.Query()["text"]
		if !ok {
			log.Fatal("text not found")
		}

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'key' is missing")
			return
		}

		// Query()["key"] will return an array of items,
		// we only want the single item.
		text := keys[0]

		ttsReq := textToSpeechRequest(text)

		resp, err := ttsClient.SynthesizeSpeech(request.Context(), ttsReq)
		if err != nil {
			log.Fatal(err)
		}

		// The resp's AudioContent is binary.
		filename := GenerateName()
		// err = ioutil.WriteFile(filename, resp.AudioContent, 0644)
		// if err != nil {
		//   log.Fatal(err)
		// }

		audioBuffer := bytes.NewReader(resp.AudioContent)

		bucket := "imre-text-to-speech"

		gcsObject := gcsClient.Bucket(bucket).Object(filename)

		// Upload an object with storage.Writer.
		wc := gcsObject.NewWriter(request.Context())
		if _, err = io.Copy(wc, audioBuffer); err != nil {
			log.Fatalf("io.Copy: %v", err)
		}
		if err := wc.Close(); err != nil {
			log.Fatalf("Writer.Close: %v", err)
		}
		log.Printf("Blob %v uploaded.\n", filename)

		acl := gcsObject.ACL()
		if err := acl.Set(request.Context(), storage.AllUsers, storage.RoleReader); err != nil {
			log.Fatalf("ACLHandle.Set: %v", err)
		}
		log.Printf("Blob %v is now publicly accessible.\n", filename)

		attrs, err := gcsObject.Attrs(request.Context())
		if err != nil {
			log.Fatalf("Object(%q).Attrs: %v", filename, err)
		}

		// msg := fmt.Sprintf("Audio content written to file: %v\n", attrs.MediaLink)
		response := map[string]string{
			"url": attrs.MediaLink,
		}

		respBytes, err := json.Marshal(response)
		if err != nil {
			log.Fatalf(err.Error())
		}

		writer.Header().Set("content-type", "application/json")
		writer.Write(respBytes)

	}
}

func textToSpeechRequest(text string) *texttospeechpb.SynthesizeSpeechRequest {
	// Perform the text-to-speech request on the text input with the selected
	// voice parameters and audio file type.
	return &texttospeechpb.SynthesizeSpeechRequest{
		// Set the text input to be synthesized.
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
		},
		// Build the voice request, select the language code ("en-US") and the SSML
		// voice gender ("neutral").
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "id-ID", // "en-US",
			SsmlGender:   texttospeechpb.SsmlVoiceGender_NEUTRAL,
		},
		// Select the type of audio file you want returned.
		AudioConfig: &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		},
	}
}
