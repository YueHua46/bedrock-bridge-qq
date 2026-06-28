package onebot

import (
	"regexp"
	"strings"
)

var cqCodeRe = regexp.MustCompile(`\[CQ:([a-zA-Z0-9_-]+)(?:,[^\]]*)?\]`)

func CleanForMinecraft(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	out := cqCodeRe.ReplaceAllStringFunc(raw, func(token string) string {
		match := cqCodeRe.FindStringSubmatch(token)
		if len(match) < 2 {
			return ""
		}
		switch strings.ToLower(match[1]) {
		case "image":
			return "[图片]"
		case "record":
			return "[语音]"
		case "video":
			return "[视频]"
		case "file":
			return "[文件]"
		case "face", "bface", "sface", "emoji":
			return "[表情]"
		case "at":
			qq := cqParam(token, "qq")
			if qq == "all" {
				return "@全体成员"
			}
			if qq != "" {
				return "@" + qq
			}
			return "@某人"
		case "reply":
			return ""
		case "json", "xml":
			return "[卡片消息]"
		case "music":
			return "[音乐]"
		case "forward":
			return "[合并转发]"
		default:
			return "[" + match[1] + "]"
		}
	})
	out = strings.Join(strings.Fields(out), " ")
	out = strings.TrimSpace(out)
	out = strings.Trim(out, "- \t\r\n")
	return out
}

func cqParam(token, key string) string {
	body := strings.TrimSuffix(strings.TrimPrefix(token, "[CQ:"), "]")
	parts := strings.Split(body, ",")
	for _, part := range parts[1:] {
		k, v, ok := strings.Cut(part, "=")
		if ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
