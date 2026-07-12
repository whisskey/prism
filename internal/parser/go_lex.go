// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package parser

import (
	"fmt"
	"go/scanner"
	"go/token"
	"strings"
)

// TokenCategory classifies tokens for display and tooling.
type TokenCategory string

const (
	TokenCategoryKeyword    TokenCategory = "KEYWORD"
	TokenCategoryIdent      TokenCategory = "IDENT"
	TokenCategoryLiteral    TokenCategory = "LITERAL"
	TokenCategoryOperator   TokenCategory = "OPERATOR"
	TokenCategoryDelimiter  TokenCategory = "DELIMITER"
	TokenCategoryAssign     TokenCategory = "ASSIGN"
	TokenCategoryComparison TokenCategory = "COMPARISON"
	TokenCategorySpecial    TokenCategory = "SPECIAL"
	TokenCategoryComment    TokenCategory = "COMMENT"
	TokenCategoryIllegal    TokenCategory = "ILLEGAL"
)

// Token is a single lexical unit from a Go source file.
type Token struct {
	Pos      token.Position
	Kind     token.Token
	Literal  string
	Category TokenCategory
}

// String returns a human-readable token description.
func (t Token) String() string {
	if t.Literal != "" && t.Literal != t.Kind.String() {
		return fmt.Sprintf("%s %q", t.Kind, t.Literal)
	}
	return t.Kind.String()
}

type lexOptions struct {
	IncludeComments bool
}

type tokenFile struct {
	Name    string
	Source  string
	Tokens  []Token
	FileSet *token.FileSet
}

func (f *tokenFile) IllegalTokens() []Token {
	if f == nil {
		return nil
	}
	var illegal []Token
	for _, tok := range f.Tokens {
		if tok.Kind == token.ILLEGAL {
			illegal = append(illegal, tok)
		}
	}
	return illegal
}

func (f *tokenFile) FormatTable() string {
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Position  Token     Literal\n")
	for _, tok := range f.Tokens {
		if tok.Kind == token.EOF {
			continue
		}
		pos := fmt.Sprintf("%d:%d", tok.Pos.Line, tok.Pos.Column)
		fmt.Fprintf(&b, "%-9s %-9s %q\n", pos, tok.Kind, tok.Literal)
	}
	return b.String()
}

func tokenizeGoSource(filename, src string, opts lexOptions) (*tokenFile, error) {
	fset := token.NewFileSet()
	return tokenizeGoFileSet(fset, filename, src, opts)
}

func tokenizeGoFileSet(fset *token.FileSet, filename, src string, opts lexOptions) (*tokenFile, error) {
	if fset == nil {
		return nil, fmt.Errorf("file set is required")
	}
	if filename == "" {
		filename = "input.go"
	}

	file := fset.AddFile(filename, fset.Base(), len(src))

	mode := scanner.Mode(0)
	if opts.IncludeComments {
		mode |= scanner.ScanComments
	}

	var s scanner.Scanner
	s.Init(file, []byte(src), nil, mode)

	tokens := make([]Token, 0, len(src)/4)
	for {
		pos, kind, lit := s.Scan()
		if kind == token.EOF {
			break
		}

		position := fset.Position(pos)
		if kind == token.SEMICOLON && lit == "\n" {
			lit = "\\n"
		}

		tokens = append(tokens, Token{
			Pos:      position,
			Kind:     kind,
			Literal:  lit,
			Category: classifyToken(kind),
		})
	}

	return &tokenFile{
		Name:    filename,
		Source:  src,
		Tokens:  tokens,
		FileSet: fset,
	}, nil
}

func classifyToken(kind token.Token) TokenCategory {
	switch {
	case kind == token.ILLEGAL:
		return TokenCategoryIllegal
	case kind == token.COMMENT:
		return TokenCategoryComment
	case kind == token.IDENT:
		return TokenCategoryIdent
	case kind.IsKeyword():
		return TokenCategoryKeyword
	case kind.IsLiteral():
		return TokenCategoryLiteral
	case isComparisonToken(kind):
		return TokenCategoryComparison
	case isAssignToken(kind):
		return TokenCategoryAssign
	case isDelimiterToken(kind):
		return TokenCategoryDelimiter
	case isSpecialToken(kind):
		return TokenCategorySpecial
	case kind.IsOperator():
		return TokenCategoryOperator
	default:
		return TokenCategoryOperator
	}
}

func isComparisonToken(k token.Token) bool {
	switch k {
	case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
		return true
	default:
		return false
	}
}

func isAssignToken(k token.Token) bool {
	switch k {
	case token.ASSIGN, token.DEFINE,
		token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN, token.QUO_ASSIGN, token.REM_ASSIGN,
		token.AND_ASSIGN, token.OR_ASSIGN, token.XOR_ASSIGN,
		token.SHL_ASSIGN, token.SHR_ASSIGN, token.AND_NOT_ASSIGN:
		return true
	default:
		return false
	}
}

func isDelimiterToken(k token.Token) bool {
	switch k {
	case token.LPAREN, token.RPAREN, token.LBRACE, token.RBRACE,
		token.LBRACK, token.RBRACK, token.COMMA, token.PERIOD, token.SEMICOLON, token.COLON:
		return true
	default:
		return false
	}
}

func isSpecialToken(k token.Token) bool {
	switch k {
	case token.ELLIPSIS, token.ARROW:
		return true
	default:
		return false
	}
}
