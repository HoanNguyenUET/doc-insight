package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Embed: tạo embedding theo lô qua batchEmbedContents.

type geminiEmbedRequest struct {
	Model   string        `json:"model"`
	Content geminiContent `json:"content"`
	// Model mới (gemini-embedding-*) mặc định nhiều chiều hơn 768.
	OutputDimensionality int `json:"outputDimensionality"`
}

type geminiEmbedResponse struct {
	Embeddings []struct {
		Values []float32 `json:"values"`
	} `json:"embeddings"`
}

func (g *GeminiProvider) EmbedModel() string { return g.embedModel }

func (g *GeminiProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += geminiMaxEmbedBatch {
		batch := texts[start:min(start+geminiMaxEmbedBatch, len(texts))]
		v, err := g.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		vecs = append(vecs, v...)
	}
	return vecs, nil
}

func (g *GeminiProvider) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	model := "models/" + g.embedModel
	reqs := make([]geminiEmbedRequest, len(texts))
	for i, t := range texts {
		reqs[i] = geminiEmbedRequest{Model: model, Content: geminiContent{Parts: []geminiPart{{Text: t}}}, OutputDimensionality: Dimensions}
	}
	body, err := json.Marshal(map[string]any{"requests": reqs})
	if err != nil {
		return nil, fmt.Errorf("gemini embed: encode: %w", err)
	}

	resp, cancel, err := g.post(ctx, "/"+model+":batchEmbedContents", body, true)
	if err != nil {
		return nil, fmt.Errorf("gemini embed: %w", err)
	}
	defer cancel()
	defer resp.Body.Close()

	var res geminiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("gemini embed: decode: %w", err)
	}
	if len(res.Embeddings) != len(texts) {
		return nil, fmt.Errorf("gemini embed: nhận %d vector cho %d văn bản", len(res.Embeddings), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for i, e := range res.Embeddings {
		if len(e.Values) != Dimensions {
			return nil, fmt.Errorf("gemini embed: vector %d chiều, cần %d (model %s)", len(e.Values), Dimensions, g.embedModel)
		}
		vecs[i] = e.Values
	}
	return vecs, nil
}
