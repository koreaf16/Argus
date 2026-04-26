// Package constants — 스피너 로딩 메시지 동사 목록.
//
// 파일 역할: REPL이 응답 대기 중 표시하는 현재진행형 동사 목록을 정의한다.
//
//	재미를 위해 다양한 동사를 포함한다.
//
// 포함 모듈:
//   - SpinnerVerbs: 현재진행형 동사 슬라이스.
//   - GetSpinnerVerbs(): 설정 오버라이드를 포함한 동사 목록 반환 (Phase 2 확장용).
//
// 호출/사용 방식:
//   - internal/repl/render.go 의 스피너 컴포넌트에서 랜덤 동사 선택 시 참조.
//
// 연결:
//   - 원본: src/constants/spinnerVerbs.ts
package constants

// SpinnerVerbs 는 응답 대기 중 스피너에 표시할 현재진행형 동사 목록이다.
var SpinnerVerbs = []string{
	"Accomplishing", "Actioning", "Actualizing", "Architecting", "Baking",
	"Beaming", "Beboppin'", "Befuddling", "Billowing", "Blanching",
	"Bloviating", "Boogieing", "Boondoggling", "Booping", "Bootstrapping",
	"Brewing", "Bunning", "Burrowing", "Calculating", "Canoodling",
	"Caramelizing", "Cascading", "Catapulting", "Cerebrating", "Channeling",
	"Channelling", "Choreographing", "Churning", "Clauding", "Coalescing",
	"Cogitating", "Combobulating", "Composing", "Computing", "Concocting",
	"Considering", "Contemplating", "Cooking", "Crafting", "Creating",
	"Crunching", "Crystallizing", "Cultivating", "Deciphering", "Deliberating",
	"Determining", "Dilly-dallying", "Discombobulating", "Doing", "Doodling",
	"Drizzling", "Ebbing", "Effecting", "Elucidating", "Embellishing",
	"Enchanting", "Envisioning", "Evaporating", "Fermenting", "Fiddle-faddling",
	"Finagling", "Flambéing", "Flibbertigibbeting", "Flowing", "Flummoxing",
	"Fluttering", "Forging", "Forming", "Frolicking", "Frosting",
	"Gallivanting", "Galloping", "Garnishing", "Generating", "Gesticulating",
	"Germinating", "Gitifying", "Grooving", "Gusting", "Harmonizing",
	"Hashing", "Hatching", "Herding", "Honking", "Hullaballooing",
	"Hyperspacing", "Ideating", "Imagining", "Improvising", "Incubating",
	"Inferring", "Infusing", "Ionizing", "Jitterbugging", "Julienning",
	"Kneading", "Leavening", "Levitating", "Lollygagging", "Manifesting",
	"Marinating", "Meandering", "Metamorphosing", "Misting", "Moonwalking",
	"Moseying", "Mulling", "Mustering", "Musing", "Nebulizing",
	"Nesting", "Newspapering", "Noodling", "Nucleating", "Orbiting",
	"Orchestrating", "Osmosing", "Perambulating", "Percolating", "Perusing",
	"Philosophising", "Photosynthesizing", "Pollinating", "Pondering", "Pontificating",
	"Pouncing", "Precipitating", "Prestidigitating", "Processing", "Proofing",
	"Propagating", "Puttering", "Puzzling", "Quantumizing", "Razzle-dazzling",
	"Razzmatazzing", "Recombobulating", "Reticulating", "Roosting", "Ruminating",
	"Sautéing", "Scampering", "Schlepping", "Scurrying", "Seasoning",
	"Shenaniganing", "Shimmying", "Simmering", "Skedaddling", "Sketching",
	"Slithering", "Smooshing", "Sock-hopping", "Spelunking", "Spinning",
	"Sprouting", "Stewing", "Sublimating", "Swirling", "Swooping",
	"Symbioting", "Synthesizing", "Tempering", "Thinking", "Thundering",
	"Tinkering", "Tomfoolering", "Topsy-turvying", "Transfiguring", "Transmuting",
	"Twisting", "Undulating", "Unfurling", "Unravelling", "Vibing",
	"Waddling", "Wandering", "Warping", "Whatchamacalliting", "Whirlpooling",
	"Whirring", "Whisking", "Wibbling", "Working", "Wrangling",
	"Zesting", "Zigzagging",
}

// GetSpinnerVerbs 는 동사 목록을 반환한다. Phase 2에서 설정 오버라이드를 지원할 예정.
func GetSpinnerVerbs() []string {
	return SpinnerVerbs
}
