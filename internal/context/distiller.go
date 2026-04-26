package context

import (
	"fmt"
	"strings"
)

const (
	// Keep small tool outputs inline.
	FullThresholdChars = 8_000

	// Head/tail slices for PARTIAL projection.
	PartialHeadLines = 80
	PartialTailLines = 60

	// Above this, attempt summary projection.
	SummaryThresholdChars = 20_000

	// Skip LLM summarization for very large payloads.
	LLMSummaryMaxChars = 250_000

	// Hard cap for inline text.
	MaxInlineChars = FullThresholdChars
)

type DistillResult struct {
	InlineText   string
	Projection   Projection
	ArtifactID   string
	ArtifactPath string
}

type Distiller struct {
	store       *ArtifactStore
	manifest    *ArtifactManifest
	summarizeFn func(toolName, content string) (string, error)
}

func NewDistiller(store *ArtifactStore, manifest *ArtifactManifest, summarizeFn func(toolName, content string) (string, error)) *Distiller {
	return &Distiller{
		store:       store,
		manifest:    manifest,
		summarizeFn: summarizeFn,
	}
}

func (d *Distiller) Distill(seq int, toolName, callID, rawOutput string, isError bool) DistillResult {
	normalized := NormalizeToolResultForContext(toolName, rawOutput)
	chars := len(normalized)

	errorPrefix := ""
	if isError && chars > 0 {
		errorPrefix = extractErrorPrefix(normalized)
	}

	if chars <= FullThresholdChars {
		return DistillResult{
			InlineText: normalized,
			Projection: ProjectionFull,
		}
	}

	artRef, err := d.store.Save(seq, toolName, callID, normalized)
	if err != nil {
		artRef = nil
	}

	artID := ""
	artPath := ""
	if artRef != nil {
		d.manifest.Add(artRef)
		artID = artRef.ID
		artPath = artRef.Path
	}

	partialText := buildPartial(normalized, errorPrefix, artPath, PartialHeadLines, PartialTailLines)
	if chars > SummaryThresholdChars {
		summaryText := d.tryExtractSummary(toolName, normalized, chars)
		if summaryText != "" {
			inline := fmt.Sprintf("%s\n\n--- Tool Output Summary ---\n%s", partialText, summaryText)
			if errorPrefix != "" && !strings.Contains(inline, errorPrefix) {
				inline = errorPrefix + "\n\n" + inline
			}
			return DistillResult{
				InlineText:   truncateInline(inline),
				Projection:   ProjectionSummary,
				ArtifactID:   artID,
				ArtifactPath: artPath,
			}
		}
	}

	return DistillResult{
		InlineText:   truncateInline(partialText),
		Projection:   ProjectionPartial,
		ArtifactID:   artID,
		ArtifactPath: artPath,
	}
}

func (d *Distiller) tryExtractSummary(toolName, content string, chars int) string {
	if d.summarizeFn != nil && chars <= LLMSummaryMaxChars {
		if s, err := d.summarizeFn(toolName, content); err == nil && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return extractiveSummary(content)
}

func buildPartial(content, errorPrefix, artifactPath string, headLines, tailLines int) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder

	if errorPrefix != "" {
		sb.WriteString(errorPrefix)
		sb.WriteString("\n\n")
	}

	headEnd := headLines
	if headEnd > len(lines) {
		headEnd = len(lines)
	}
	sb.WriteString(strings.Join(lines[:headEnd], "\n"))

	if len(lines) > headLines+tailLines {
		omitted := len(lines) - headLines - tailLines
		sb.WriteString(fmt.Sprintf("\n... (%d lines omitted) ...\n", omitted))

		tailStart := len(lines) - tailLines
		sb.WriteString(strings.Join(lines[tailStart:], "\n"))
	}

	if artifactPath != "" {
		sb.WriteString(fmt.Sprintf("\n\n[Full output: %s]", artifactPath))
	}

	return sb.String()
}

func extractErrorPrefix(content string) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) == "" && sb.Len() > 0 {
			break
		}
		sb.WriteString(line)
		sb.WriteString("\n")
		if sb.Len() > 500 {
			break
		}
	}
	return strings.TrimSpace(sb.String())
}

func extractiveSummary(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	const n = 5
	if len(lines) <= n*2 {
		return content
	}
	head := lines[:n]
	tail := lines[len(lines)-n:]
	return strings.Join(head, "\n") + "\n...\n" + strings.Join(tail, "\n")
}

func truncateInline(s string) string {
	if len(s) <= MaxInlineChars {
		return s
	}
	return s[:MaxInlineChars] + "\n...[truncated]"
}
