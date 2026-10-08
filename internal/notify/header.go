package notify

import (
	"mime"
	"strings"
)

// encodeHeader makes a header value safe to send.
//
// Two jobs, and either one of them happens to do the other's. A non-ASCII
// subject has to be encoded or it arrives as mojibake, and a newline in a value
// would end the header and let whatever follows be read as another one, which
// is how a subject becomes a Bcc.
//
// Measured rather than assumed: with only the replacer the injection is stopped,
// and with only the encoding it is stopped too, because Q-encoding escapes
// control characters along with everything else it escapes. Both are kept —
// the replacer because stopping it should not depend on a side effect of
// something done for another reason, and the encoding because mojibake is the
// job it is actually here for.
func encodeHeader(value string) string {
	cleaned := strings.NewReplacer("\r", " ", "\n", " ").Replace(value)

	return mime.QEncoding.Encode("utf-8", cleaned)
}
