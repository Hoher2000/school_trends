// tts/salutespeech.go
package tts

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	synthesis "github.com/Hoher2000/school_trends/tts/api/synthesis/v1"
)

type SaluteClient struct {
	tokenManager *TokenManager
	conn         *grpc.ClientConn
	client       synthesis.SmartSpeechClient
}

func NewSaluteClient(tokenManager *TokenManager) (*SaluteClient, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.NewClient(
		"smartspeech.sber.ru:443",
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc.NewClient: %w", err)
	}
	return &SaluteClient{
		tokenManager: tokenManager,
		conn:         conn,
		client:       synthesis.NewSmartSpeechClient(conn),
	}, nil
}

func (sc *SaluteClient) Close() error {
	sc.tokenManager.Stop()
	return sc.conn.Close()
}

func (sc *SaluteClient) Synthesize(text string) (string, error) {
	// Получаем актуальный токен
	token := sc.tokenManager.GetToken()

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	stream, err := sc.client.Synthesize(ctx, &synthesis.SynthesisRequest{
		Text:          text,
		AudioEncoding: synthesis.SynthesisRequest_WAV,
		Language:      "ru-RU",
		ContentType:   synthesis.SynthesisRequest_TEXT,
		Voice:         "Nec_24000",
	})
	if err != nil {
		return "", fmt.Errorf("Synthesize: %w", err)
	}

	var audioData []byte
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("stream error: %w", err)
		}
		audioData = append(audioData, resp.Data...)
		if resp.AudioDuration != nil {
			log.Printf("Синтезировано аудио: %v", resp.AudioDuration.AsDuration())
		}
	}

	outputDir := "audio"
	os.MkdirAll(outputDir, 0755)
	filename := fmt.Sprintf("audio_salute_%s_%d.wav", time.Now().Format("150405"), time.Now().UnixNano()%1000)
	fullPath := filepath.Join(outputDir, filename)
	if err := os.WriteFile(fullPath, audioData, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fullPath, nil
}
