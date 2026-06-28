package onebot

import "testing"

func TestCleanForMinecraft(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text",
			in:   "哈喽",
			want: "哈喽",
		},
		{
			name: "image url stripped",
			in:   "哈喽[CQ:image,file=a.png,url=https://example.com/a.png]",
			want: "哈喽[图片]",
		},
		{
			name: "record and video",
			in:   "[CQ:record,file=a.amr,url=http://x] [CQ:video,file=v.mp4,url=http://x]",
			want: "[语音] [视频]",
		},
		{
			name: "at all",
			in:   "[CQ:at,qq=all] 集合",
			want: "@全体成员 集合",
		},
		{
			name: "reply removed",
			in:   "[CQ:reply,id=123]收到",
			want: "收到",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanForMinecraft(tt.in); got != tt.want {
				t.Fatalf("CleanForMinecraft() = %q, want %q", got, tt.want)
			}
		})
	}
}
