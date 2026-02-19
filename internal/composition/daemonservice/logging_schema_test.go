package daemonservice

import "testing"

func TestMessageCorrelationID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		messageID string
		contactID string
		want      string
	}{
		{name: "both ids", messageID: "m1", contactID: "c1", want: "c1:m1"},
		{name: "message only", messageID: "m1", contactID: "", want: "m1"},
		{name: "contact only", messageID: "", contactID: "c1", want: "c1"},
		{name: "none", messageID: "", contactID: "", want: "n/a"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := messageCorrelationID(tc.messageID, tc.contactID)
			if got != tc.want {
				t.Fatalf("unexpected correlation id: got=%q want=%q", got, tc.want)
			}
		})
	}
}
