// Package constants — Anthropic API 크기 제한.
//
// 파일 역할: API가 서버 측에서 강제하는 이미지·PDF·미디어 한도 상수.
//
//	개별 도구나 서비스가 요청 크기를 사전 검증할 때 참조한다.
//
// 포함 모듈:
//   - API_IMAGE_MAX_BASE64_SIZE: base64 이미지 최대 크기 (5 MB)
//   - IMAGE_TARGET_RAW_SIZE: base64 인코딩 전 권장 원본 크기
//   - IMAGE_MAX_WIDTH / IMAGE_MAX_HEIGHT: 클라이언트 리사이즈 한도
//   - PDF_TARGET_RAW_SIZE: PDF 원본 최대 크기 (20 MB)
//   - API_PDF_MAX_PAGES: API가 허용하는 최대 PDF 페이지 수
//   - PDF_* 관련 파생 상수들
//   - API_MAX_MEDIA_PER_REQUEST: 요청당 미디어 최대 수
//
// 호출/사용 방식:
//   - internal/tools/bash, internal/services/api 등에서 요청 크기 사전 검증 시 참조.
//
// 연결:
//   - 원본: src/constants/apiLimits.ts
package constants

// API 이미지 base64 최대 크기 (5 MB). API가 이 값을 초과하는 이미지를 거부한다.
const APIImageMaxBase64Size = 5 * 1024 * 1024

// base64 한도 이하로 맞추기 위한 원본 이미지 권장 최대 크기 (3.75 MB).
const ImageTargetRawSize = APIImageMaxBase64Size * 3 / 4

// 클라이언트 리사이즈 최대 해상도.
const ImageMaxWidth = 2000
const ImageMaxHeight = 2000

// PDF 요청 한도 이하로 맞추기 위한 원본 PDF 권장 최대 크기 (20 MB).
const PDFTargetRawSize = 20 * 1024 * 1024

// API가 허용하는 최대 PDF 페이지 수.
const APIPDFMaxPages = 100

// 이 크기를 초과하는 PDF는 base64 문서 블록 대신 페이지 이미지로 변환 (3 MB).
const PDFExtractSizeThreshold = 3 * 1024 * 1024

// 페이지 추출 경로에서 허용하는 PDF 최대 크기 (100 MB).
const PDFMaxExtractSize = 100 * 1024 * 1024

// Read 도구가 pages 파라미터로 한 번에 추출할 수 있는 최대 페이지 수.
const PDFMaxPagesPerRead = 20

// @ 멘션 시 컨텍스트 인라인 처리 임계 페이지 수 (초과 시 참조 처리).
const PDFAtMentionInlineThreshold = 10

// 요청당 미디어(이미지+PDF) 최대 수. 초과 시 명확한 오류 제공을 위해 클라이언트에서 사전 검증.
const APIMaxMediaPerRequest = 100
