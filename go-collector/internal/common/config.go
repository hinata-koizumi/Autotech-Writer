package common

import "strings"

// --- Breaking News Configuration ---

// BreakingNewsScoreThreshold is the minimum score for an item to be flagged as breaking news.
const BreakingNewsScoreThreshold = 50

// --- Compliance (NG Keywords) ---

var EnglishNGKeywords = []string{
	"war", "invasion", "sanctions", "dictatorship",
	"genocide", "terrorism", "nuclear weapon",
	"coup", "military strike",
}

var JapaneseNGKeywords = []string{
	"戦争", "紛争", "侵攻", "侵略", "制裁",
	"政権", "独裁", "弾圧", "虐殺", "テロ",
	"核兵器", "ミサイル", "軍事", "武装",
	"選挙不正", "クーデター", "暴動",
}

// --- ArXiv Tiers ---

var ArxivTier1 = []string{"google research", "deepmind", "openai", "anthropic", "meta ai", "fundamental ai research", "fair"}
var ArxivTier2 = []string{"mit", "stanford", "berkeley", "carnegie mellen", "cmu", "microsoft research", "allen institute", "ai2"}
var ArxivTier3 = []string{"tsinghua", "mila", "vector institute", "mistral"}

// --- Top Conferences ---

var TopConferencePatterns = map[string]string{
	"neurips": `(?i)(accepted\s+(to|at|for)\s+)?\b(NeurIPS|NIPS)\b(\s*20[0-9]{2})?`,
	"iclr":    `(?i)(accepted\s+(to|at|for)\s+)?\bICLR\b(\s*20[0-9]{2})?`,
	"icml":    `(?i)(accepted\s+(to|at|for)\s+)?\bICML\b(\s*20[0-9]{2})?`,
	"cvpr":    `(?i)(accepted\s+(to|at|for)\s+)?\bCVPR\b(\s*20[0-9]{2})?`,
	"acl":     `(?i)(accepted\s+(to|at|for)\s+)?\bACL\b(\s*20[0-9]{2})?`,
	"emnlp":   `(?i)(accepted\s+(to|at|for)\s+)?\bEMNLP\b(\s*20[0-9]{2})?`,
	"kdd":     `(?i)(accepted\s+(to|at|for)\s+)?\bKDD\b(\s*20[0-9]{2})?`,
	"sigir":   `(?i)(accepted\s+(to|at|for)\s+)?\bSIGIR\b(\s*20[0-9]{2})?`,
	"icse":    `(?i)(accepted\s+(to|at|for)\s+)?\bICSE\b(\s*20[0-9]{2})?`,
	"fse":     `(?i)(accepted\s+(to|at|for)\s+)?\bFSE\b(\s*20[0-9]{2})?`,
	"ase":     `(?i)(accepted\s+(to|at|for)\s+)?\bASE\b(\s*20[0-9]{2})?`,
	"issta":   `(?i)(accepted\s+(to|at|for)\s+)?\bISSTA\b(\s*20[0-9]{2})?`,
}

// --- GitHub Tiers ---

var GitHubTier1 = map[string]bool{
	"google": true, "openai": true, "microsoft": true, "facebook": true,
	"meta": true, "anthropic": true, "deepmind": true, "nvidia": true,
	"apple": true, "aws": true, "huggingface": true,
}

var GitHubTier2 = map[string]bool{
	"hashicorp": true, "kubernetes": true, "apache": true, "replicate": true,
	"mistralai": true, "pytorch": true, "tensorflow": true, "cohere-ai": true,
	"langchain-ai": true, "vercel": true, "vllm-project": true, "ollama": true,
	"ultralytics": true, "tiangolo": true, "docker": true,
}

var GitHubTier3 = map[string]bool{
	"aws-samples": true, "google-research": true, "microsoft-research": true,
	"google-cloudplatform": true, "azure": true,
}

// IsRenownedOwner returns a score and true if the owner is recognized.
func IsRenownedOwner(owner string) (int, bool) {
	owner = strings.ToLower(owner)
	if GitHubTier1[owner] {
		return 50, true
	}
	if GitHubTier2[owner] {
		return 30, true
	}
	if GitHubTier3[owner] {
		return 10, true
	}
	return 0, false
}
