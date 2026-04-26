// 파일 역할: TokenEstimator — chars→token 추정, 모델별 context window 관리.
package context

import "unicode"

const (
	// ReservedOutputFraction 은 어시스턴트 출력을 위해 예약할 context 비율이다 (20%).
	ReservedOutputFraction = 0.20
)

// TokenEstimator 는 텍스트와 노드 목록의 토큰 수를 추정한다.
type TokenEstimator struct{}

// NewTokenEstimator 는 새 TokenEstimator를 반환한다.
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{}
}

// estimateRunes 는 텍스트를 ASCII와 CJK 계열로 분리해 토큰 수를 추정한다.
// - ASCII/Latin: ~4 chars/token (BPE 기준)
// - CJK(한글·한자·일본어 등): ~1.5 chars/token (한 글자 = 1~2 토큰)
//
// CJK를 단순 chars/4로 추정하면 최대 2.5배 과소 추정되어 context overflow 유발.
func estimateRunes(text string) int {
	var ascii, cjk int
	for _, r := range text {
		if r < 0x80 {
			ascii++
		} else if isCJK(r) {
			cjk++
		} else {
			ascii++ // 기타 유니코드는 보수적으로 ASCII 취급
		}
	}
	if ascii == 0 && cjk == 0 {
		return 0
	}
	tokens := ascii/4 + (cjk*2+1)/3 // cjk: ceil(cjk/1.5) ≈ cjk*2/3
	if tokens == 0 {
		return 1
	}
	return tokens
}

// isCJK 는 룬이 CJK·한글·가나 계열 문자인지 판별한다.
func isCJK(r rune) bool {
	return unicode.In(r,
		unicode.Hangul,
		unicode.Han,
		unicode.Hiragana,
		unicode.Katakana,
	)
}

// EstimateText 는 텍스트 문자열의 추정 토큰 수를 반환한다.
func (e *TokenEstimator) EstimateText(text string) int {
	return estimateRunes(text)
}

// EstimateNodes 는 노드 목록의 총 추정 토큰 수를 반환한다.
func (e *TokenEstimator) EstimateNodes(nodes []*Node) int {
	total := 0
	for _, nd := range nodes {
		total += nd.EstimateTokens()
	}
	return total
}

// BudgetFor 는 주어진 context window에서 메시지 graph에 할당 가능한 토큰 예산을 반환한다.
//
//	budget = contextWindow - systemTokens - reservedOutput - currentUserTokens
//
// currentUserChars 는 사용자 입력의 바이트 수다. 내용을 알 수 없으므로
// 보수적으로 3 bytes/token (혼합 언어 기준)을 적용한다.
func (e *TokenEstimator) BudgetFor(contextWindow, systemTokens, currentUserChars int) int {
	reserved := int(float64(contextWindow) * ReservedOutputFraction)
	// 3 bytes/token: ASCII(4 bytes)보다 보수적, CJK(1.5 chars)보다 여유 있는 중간값
	userTokens := (currentUserChars + 2) / 3
	budget := contextWindow - systemTokens - reserved - userTokens
	if budget < 0 {
		return 0
	}
	return budget
}
