package telegram

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestTelegramIncidentIsHTML(t *testing.T) {
	text := incidentText(providertest.Rich(), telegramTextLimit)
	require.Equal(t, "🔴 <b>payments</b> in <b>shop</b> is crash-looping.\n"+
		"It said <code>boom *x* @​channel</code>.\n"+
		"Service <b>web</b> can&#39;t serve traffic. Run\n"+
		"<pre>kubectl logs payments -n shop</pre>", text)
}
