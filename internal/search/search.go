package search

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/JLugagne/bm25"
	"github.com/JLugagne/sandwarden/internal/store"
)

// Kind identifies the catalog an indexed document comes from.
type Kind string

// Indexed document kinds.
const (
	KindSkill   Kind = "skill"
	KindCommand Kind = "command"
	KindKit     Kind = "kit"
)

// Document is one indexable catalog entry: a skill, a command or a kit. Body
// carries the text read from the checkout (SKILL.md, the command file or
// spec.yaml); it is indexed but never returned to the UI.
type Document struct {
	Kind        Kind
	ID          int64
	StoreID     int64
	StoreName   string
	Name        string
	DisplayName string
	Description string
	Plugin      string
	KitKind     string
	Body        string
}

// Result is one ranked hit.
type Result struct {
	Kind        Kind    `json:"kind"`
	ID          int64   `json:"id"`
	StoreID     int64   `json:"store_id"`
	StoreName   string  `json:"store_name"`
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name,omitempty"`
	Description string  `json:"description"`
	Plugin      string  `json:"plugin,omitempty"`
	KitKind     string  `json:"kit_kind,omitempty"`
	Score       float64 `json:"score"`
	Snippet     string  `json:"snippet,omitempty"`
}

// Okapi parameters and snippet limits.
const (
	minQueryRunes = 2
	okapiK1       = 1.5
	okapiB        = 0.75
	maxBodyRunes  = 20000
	snippetRunes  = 160
)

// Index is an immutable BM25 index over skill and kit documents. It is safe
// for concurrent queries.
type Index struct {
	bm    *bm25.BM25Okapi
	docs  []Document
	vocab []string
}

// Build lists the skill and kit catalogs from the store and indexes their
// metadata and document bodies.
func Build(ctx context.Context, st *store.Store) (*Index, error) {
	docs, err := Collect(ctx, st)
	if err != nil {
		return nil, err
	}
	return New(docs)
}

// New builds an index over the given documents. Documents whose text yields
// no token are skipped.
func New(docs []Document) (*Index, error) {
	corpus := make([]string, 0, len(docs))
	indexed := make([]Document, 0, len(docs))
	vocabulary := make(map[string]struct{})
	for _, doc := range docs {
		text := doc.searchText()
		tokens := Tokenize(text)
		if len(tokens) == 0 {
			continue
		}
		corpus = append(corpus, text)
		indexed = append(indexed, doc)
		for _, token := range tokens {
			vocabulary[token] = struct{}{}
		}
	}
	if len(corpus) == 0 {
		return &Index{}, nil
	}
	engine, err := bm25.NewBM25Okapi(corpus, Tokenize, okapiK1, okapiB, nil)
	if err != nil {
		return nil, err
	}
	vocab := make([]string, 0, len(vocabulary))
	for token := range vocabulary {
		vocab = append(vocab, token)
	}
	sort.Strings(vocab)
	return &Index{bm: engine, docs: indexed, vocab: vocab}, nil
}

// Search returns the documents that match query, best first. Terms match
// whole words and, from two characters on, word prefixes, so type-ahead
// ("kub") already finds "kubernetes". A blank or one-character query returns
// no results.
// Search returns the documents that match query, best first. Terms match
// whole words and, from two characters on, word prefixes, so type-ahead
// ("kub") already finds "kubernetes". A blank or one-character query returns
// no results.
func (ix *Index) Search(query string, limit int) []Result {
	if ix == nil || ix.bm == nil {
		return nil
	}
	if utf8.RuneCountInString(strings.TrimSpace(query)) < minQueryRunes {
		return nil
	}
	terms := ix.expand(Tokenize(query))
	if len(terms) == 0 {
		return nil
	}
	scores, err := ix.bm.GetScores(terms)
	if err != nil {
		return nil
	}
	ranked := make([]int, 0, len(scores))
	for i, score := range scores {
		if score > 0 {
			ranked = append(ranked, i)
		}
	}
	if len(ranked) == 0 {
		return ix.fallback(terms, limit)
	}
	sort.SliceStable(ranked, func(i, j int) bool { return scores[ranked[i]] > scores[ranked[j]] })
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	results := make([]Result, 0, len(ranked))
	for _, i := range ranked {
		results = append(results, ix.result(i, scores[i], terms))
	}
	return results
}

// expand adds every indexed word that extends a query term, so partial words
// search like type-ahead. Terms shorter than two characters match exactly.
func (ix *Index) expand(terms []string) []string {
	seen := make(map[string]struct{}, len(terms))
	expanded := make([]string, 0, len(terms))
	add := func(term string) {
		if _, ok := seen[term]; ok {
			return
		}
		seen[term] = struct{}{}
		expanded = append(expanded, term)
	}
	for _, term := range terms {
		add(term)
		if utf8.RuneCountInString(term) < 2 {
			continue
		}
		for _, word := range ix.vocab {
			if len(word) > len(term) && strings.HasPrefix(word, term) {
				add(word)
			}
		}
	}
	return expanded
}

// searchText is the indexed representation of one document. The name,
// display name and store are repeated so a metadata match outranks a
// body-only match.
func (d Document) searchText() string {
	var builder strings.Builder
	boost := strings.TrimSpace(strings.Join([]string{d.Name, d.DisplayName, d.StoreName}, "\n"))
	for range 3 {
		builder.WriteString(boost)
		builder.WriteString("\n")
	}
	builder.WriteString(d.Description)
	builder.WriteString("\n")
	builder.WriteString(d.Plugin)
	builder.WriteString("\n")
	builder.WriteString(d.Body)
	return builder.String()
}

// snippet returns a short window of the document body around the first
// matched term, or an empty string when the match is not in the body.
func (d Document) snippet(terms []string) string {
	body := strings.Join(strings.Fields(d.Body), " ")
	if body == "" {
		return ""
	}
	runes := []rune(body)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	haystack := string(lower)
	best := -1
	for _, term := range terms {
		index := strings.Index(haystack, term)
		if index < 0 {
			continue
		}
		position := utf8.RuneCountInString(haystack[:index])
		if best < 0 || position < best {
			best = position
		}
	}
	if best < 0 {
		return ""
	}
	start := best - snippetRunes/3
	if start < 0 {
		start = 0
	}
	end := start + snippetRunes
	if end > len(runes) {
		end = len(runes)
	}
	text := string(runes[start:end])
	if start > 0 {
		text = "…" + text
	}
	if end < len(runes) {
		text += "…"
	}
	return strings.TrimSpace(text)
}

// Collect lists every skill, command and kit with the body read from its
// checkout. Missing or unreadable files only cost the body.
func Collect(ctx context.Context, st *store.Store) ([]Document, error) {
	skillStores, err := st.ListSkillStores(ctx)
	if err != nil {
		return nil, err
	}
	skills, err := st.ListAllSkillItems(ctx)
	if err != nil {
		return nil, err
	}
	kitStores, err := st.ListKitStores(ctx)
	if err != nil {
		return nil, err
	}
	kits, err := st.ListAllKitItems(ctx)
	if err != nil {
		return nil, err
	}
	skillPaths := make(map[int64]string, len(skillStores))
	for _, item := range skillStores {
		skillPaths[item.ID] = item.Path
	}
	kitPaths := make(map[int64]string, len(kitStores))
	for _, item := range kitStores {
		kitPaths[item.ID] = item.Path
	}
	docs := make([]Document, 0, len(skills)+len(kits))
	for _, item := range skills {
		docs = append(docs, Document{
			Kind:        Kind(item.Kind),
			ID:          item.ID,
			StoreID:     item.StoreID,
			StoreName:   item.StoreName,
			Name:        item.Name,
			Description: item.Description,
			Plugin:      item.Plugin,
			Body:        skillBody(skillPaths[item.StoreID], item.RelPath, item.Kind),
		})
	}
	for _, item := range kits {
		docs = append(docs, Document{
			Kind:        KindKit,
			ID:          item.ID,
			StoreID:     item.StoreID,
			StoreName:   item.StoreName,
			Name:        item.Name,
			DisplayName: item.DisplayName,
			Description: item.Description,
			KitKind:     item.Kind,
			Body:        readBody(filepath.Join(kitPaths[item.StoreID], filepath.FromSlash(item.RelPath), "spec.yaml")),
		})
	}
	return docs, nil
}

// skillBody reads the markdown of one skill or command. Skill items point at
// their directory (SKILL.md inside), commands at the markdown file itself.
func skillBody(root, relPath, kind string) string {
	if root == "" || relPath == "" {
		return ""
	}
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if kind == string(KindSkill) {
		path = filepath.Join(path, "SKILL.md")
	}
	return readBody(path)
}

// readBody reads a checkout file, truncating it to maxBodyRunes and dropping
// invalid UTF-8.
func readBody(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.ToValidUTF8(string(data), "")
	runes := []rune(text)
	if len(runes) > maxBodyRunes {
		text = string(runes[:maxBodyRunes])
	}
	return text
}

// Tokenize lowercases text and splits it on every non letter or digit rune.
func Tokenize(text string) []string {
	tokens := make([]string, 0, 32)
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(string(current)))
		current = current[:0]
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

// result builds the UI payload for one indexed document.
func (ix *Index) result(index int, score float64, terms []string) Result {
	doc := ix.docs[index]
	return Result{
		Kind:        doc.Kind,
		ID:          doc.ID,
		StoreID:     doc.StoreID,
		StoreName:   doc.StoreName,
		Name:        doc.Name,
		DisplayName: doc.DisplayName,
		Description: doc.Description,
		Plugin:      doc.Plugin,
		KitKind:     doc.KitKind,
		Score:       score,
		Snippet:     doc.snippet(terms),
	}
}

// fallback matches raw substrings when BM25 has nothing to rank. Okapi drops
// terms whose IDF is zero, so a query made only of terms present in every
// document (a one-item catalog, for example) would otherwise return nothing.
func (ix *Index) fallback(terms []string, limit int) []Result {
	type hit struct {
		index int
		score int
	}
	hits := make([]hit, 0, len(ix.docs))
	for i, doc := range ix.docs {
		text := strings.ToLower(doc.searchText())
		score := 0
		for _, term := range terms {
			if strings.Contains(text, term) {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, hit{index: i, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].index < hits[j].index
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	results := make([]Result, 0, len(hits))
	for _, h := range hits {
		results = append(results, ix.result(h.index, float64(h.score), terms))
	}
	return results
}
