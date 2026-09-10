package cli

import (
	"strings"
	"testing"
)

func TestRelayedConsentSpanishLocalizesHumanFieldsWithoutChangingMachineTokens(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	question := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-consent-spanish", "--locale", "es", "--consent", "relay",
	})).Bytes())
	if question.Headline != "Gentle AI puede revisar este cambio antes de que lo des por terminado." ||
		question.Value != "La revisión lleva un poco más de tiempo y hace que el resultado sea considerablemente más seguro." ||
		question.Reason != "La revisión puede ayudar a detectar problemas de ejecución en estos cambios." ||
		strings.Contains(question.Reason, "scripts/deploy.sh") {
		t.Fatalf("Spanish consent envelope did not localize its generic reason and brief benefit: %#v", question)
	}
	if len(question.Choices) != 2 || question.Choices[0].Answer != "granted" || question.Choices[1].Answer != "declined" ||
		question.Choices[0].Label != "Revisar este cambio" || question.Choices[0].Effect != "Revisa solo este cambio; los cambios posteriores de riesgo medio o alto vuelven a pedir confirmación y la entrega requiere otra aprobación." ||
		question.Choices[1].Label != "Omitir esta vez" || question.Choices[1].Effect != "Omite solo este cambio; no crea un registro de revisión y las revisiones futuras siguen activas." {
		t.Fatalf("Spanish consent envelope changed machine answers or concise informed-consent copy: %#v", question.Choices)
	}
	for _, choice := range question.Choices {
		if !strings.Contains(choice.Invocation, "--target "+question.TargetIdentity) ||
			!strings.Contains(choice.Invocation, "--consent "+choice.Answer) ||
			!strings.Contains(choice.Invocation, "--locale es") {
			t.Fatalf("Spanish consent invocation changed its machine binding: %#v", choice)
		}
	}
	if question.OffPath.Command != reviewConsentOffPathCommand || !strings.Contains(question.OffPath.Note, "desactivar") {
		t.Fatalf("Spanish consent off path = %#v", question.OffPath)
	}
}
