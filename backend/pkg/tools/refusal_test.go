package tools

import (
	"errors"
	"fmt"
	"testing"
)

func TestRefusalText_AnswersARefusalAndFailsEverythingElse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		want     string
		refusing bool
	}{
		{
			name:     "plain refusal",
			err:      Refuse("镜像 %s 不在目标机上", "alpine:latest"),
			want:     "镜像 alpine:latest 不在目标机上",
			refusing: true,
		},
		{
			name:     "wrapped refusal",
			err:      fmt.Errorf("采集容器：%w", Refuse("操作员没有放开出网")),
			want:     "操作员没有放开出网",
			refusing: true,
		},
		{
			name:     "ordinary failure",
			err:      errors.New("connection refused"),
			refusing: false,
		},
		{
			name:     "no error at all",
			err:      nil,
			refusing: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, refused := refusalText(tc.err)
			if refused != tc.refusing {
				t.Fatalf("refused = %v, want %v", refused, tc.refusing)
			}
			if got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
		})
	}
}
