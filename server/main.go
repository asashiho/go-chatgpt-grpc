package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	chat "server/chat"

	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/grpc"
)

type server struct {
	chat.UnimplementedChatServiceServer
	aiClient *openai.Client
}

func (s *server) Chat(stream chat.ChatService_ChatServer) error {
	ctx := context.Background()
	for {
		req, err := stream.Recv()

		if err != nil {
			// Client stream might have closed. Log it and return.
			log.Printf("Failed to receive from client: %v", err)
			return err
		}
		fmt.Printf("Received request: %s\n", req.Content)

		aiReq := openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: req.Content,
				},
			},
			Stop:   []string{"\n"},
			Stream: true,
		}
		aiStream, err := s.aiClient.CreateChatCompletionStream(ctx, aiReq)
		if err != nil {
			log.Printf("Failed to create chat stream: %v", err)
			return err
		}

		defer aiStream.Close()

		for {
			aiRes, err := aiStream.Recv()

			if errors.Is(err, io.EOF) {
				log.Printf("\nStream finished.")
				break
			}

			if err != nil {
				log.Printf("Stream error: %v", err)
				break
			}

			if len(aiRes.Choices) == 0 {
				continue
			}

			res := &chat.Message{Content: aiRes.Choices[0].Delta.Content}
			log.Printf(aiRes.Choices[0].Delta.Content)

			if err := stream.Send(res); err != nil {
				log.Printf("Failed to send to client: %v", err)
				return err
			}
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	AOAI_KEY := os.Getenv("AOAI_KEY")
	AOAI_ENDPOINT := os.Getenv("AOAI_ENDPOINT")
	AOAI_VERSION := os.Getenv("AOAI_VERSION")
	AOAI_DEPLOYNAME := os.Getenv("AOAI_DEPLOYNAME")

	config := openai.DefaultAzureConfig(AOAI_KEY, AOAI_ENDPOINT)
	config.APIVersion = AOAI_VERSION
	config.AzureModelMapperFunc = func(model string) string {
		modelmapper := map[string]string{
			"gpt-3.5-turbo": AOAI_DEPLOYNAME,
		}
		if val, ok := modelmapper[model]; ok {
			return val
		}
		return model
	}

	aiClient := openai.NewClientWithConfig(config)

	s := grpc.NewServer()
	chat.RegisterChatServiceServer(s, &server{
		aiClient: aiClient,
	})

	fmt.Println("Server listening on port :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
