// Package untagged é o fixture do teste de tags: cada tipo aqui tem um campo
// que o verificador precisa apontar.
package untagged

import "github.com/marquinhosvideo/processosplatdistr/internal/audit"

// Pelo sufixo Snapshot.
type candidateSnapshot struct {
	ID       int64 `audit:"plain"`
	FullName string
	internal string
}

// Por ter tag audit em algum campo; o valor da outra é inválido.
type quarterState struct {
	Status string `audit:"plain"`
	Notes  string `audit:"secreto"`
}

// Por aparecer como Before/After de audit.Entry.
type trainingRow struct {
	Cost int64 `json:"costCents"`
}

type okSnapshot struct {
	ID   int64  `audit:"plain"`
	Body string `audit:"redacted"`
}

func record() audit.Entry {
	_ = candidateSnapshot{internal: "x"}
	_ = quarterState{}
	return audit.Entry{Action: "x", EntityType: "y", Before: &trainingRow{}, After: okSnapshot{}}
}
