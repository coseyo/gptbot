package gptbot

import (
	"strings"

	"github.com/yanyiwu/gojieba"
)

type WordSegment struct {
	Jb *gojieba.Jieba
}

func NewWordSegment(paths ...string) *WordSegment {
	return &WordSegment{
		Jb: gojieba.NewJieba(paths...),
	}
}

func (t *WordSegment) Segment(text string) string {
	return strings.Join(t.Jb.Cut(text, true), " ")
}
