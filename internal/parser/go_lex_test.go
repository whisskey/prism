// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package parser

import (
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenizeGoSource_RouteCall(t *testing.T) {
	src := `app.Get("/users/:id", getUserHandler)`
	file, err := tokenizeGoSource("main.go", src, lexOptions{})
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(file.Tokens), 8)

	assert.Equal(t, token.IDENT, file.Tokens[0].Kind)
	assert.Equal(t, "app", file.Tokens[0].Literal)
	assert.Equal(t, TokenCategoryIdent, file.Tokens[0].Category)
	assert.Equal(t, 1, file.Tokens[0].Pos.Line)

	assert.Equal(t, token.PERIOD, file.Tokens[1].Kind)
	assert.Equal(t, TokenCategoryDelimiter, file.Tokens[1].Category)

	assert.Equal(t, token.IDENT, file.Tokens[2].Kind)
	assert.Equal(t, "Get", file.Tokens[2].Literal)

	assert.Equal(t, token.LPAREN, file.Tokens[3].Kind)

	assert.Equal(t, token.STRING, file.Tokens[4].Kind)
	assert.Equal(t, `"/users/:id"`, file.Tokens[4].Literal)
	assert.Equal(t, TokenCategoryLiteral, file.Tokens[4].Category)

	assert.Equal(t, token.COMMA, file.Tokens[5].Kind)

	assert.Equal(t, token.IDENT, file.Tokens[6].Kind)
	assert.Equal(t, "getUserHandler", file.Tokens[6].Literal)

	assert.Equal(t, token.RPAREN, file.Tokens[7].Kind)
}

func TestTokenizeGoSource_SemicolonInsertion(t *testing.T) {
	src := "package main\n"
	file, err := tokenizeGoSource("main.go", src, lexOptions{})
	require.NoError(t, err)

	var semis []Token
	for _, tok := range file.Tokens {
		if tok.Kind == token.SEMICOLON {
			semis = append(semis, tok)
		}
	}
	require.NotEmpty(t, semis)
	assert.Equal(t, TokenCategoryDelimiter, semis[len(semis)-1].Category)
}

func TestTokenizeGoSource_SkipsCommentsByDefault(t *testing.T) {
	src := "package main // comment\n"
	file, err := tokenizeGoSource("main.go", src, lexOptions{})
	require.NoError(t, err)

	for _, tok := range file.Tokens {
		assert.NotEqual(t, token.COMMENT, tok.Kind)
	}
}

func TestTokenizeGoSource_IncludeComments(t *testing.T) {
	src := "package main // comment\n"
	file, err := tokenizeGoSource("main.go", src, lexOptions{IncludeComments: true})
	require.NoError(t, err)

	foundComment := false
	for _, tok := range file.Tokens {
		if tok.Kind == token.COMMENT {
			foundComment = true
			assert.Equal(t, TokenCategoryComment, tok.Category)
		}
	}
	assert.True(t, foundComment)
}

func TestTokenizeGoSource_IllegalCharacter(t *testing.T) {
	src := "package main\nvar x = $\n"
	file, err := tokenizeGoSource("main.go", src, lexOptions{})
	require.NoError(t, err)

	illegal := file.IllegalTokens()
	require.NotEmpty(t, illegal)
	assert.Equal(t, TokenCategoryIllegal, illegal[0].Category)
}

func TestClassifyToken(t *testing.T) {
	assert.Equal(t, TokenCategoryKeyword, classifyToken(token.PACKAGE))
	assert.Equal(t, TokenCategoryAssign, classifyToken(token.DEFINE))
	assert.Equal(t, TokenCategoryComparison, classifyToken(token.EQL))
	assert.Equal(t, TokenCategorySpecial, classifyToken(token.ELLIPSIS))
}

func TestTokenFileFormatTable(t *testing.T) {
	src := `app.Get("x")`
	file, err := tokenizeGoSource("main.go", src, lexOptions{})
	require.NoError(t, err)

	table := file.FormatTable()
	assert.Contains(t, table, "Position")
	assert.Contains(t, table, "IDENT")
	assert.Contains(t, table, `"app"`)
}
