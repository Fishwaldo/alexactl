package amzconst

import "time"

// SequenceBatchDelay is the debounce window for batching sequence
// operations into a single /api/behaviors/preview POST. Mirrors upstream
// SEQUENCE_BATCH_DELAY = 0.3 seconds in const/metadata.py.
const SequenceBatchDelay = 300 * time.Millisecond

// JSON @type discriminator strings used in the sequence wire format.
// Ported from upstream sequence.py.
const (
	SequenceTypeAttr  = "com.amazon.alexa.behaviors.model.Sequence"
	SerialNodeAttr    = "com.amazon.alexa.behaviors.model.SerialNode"
	ParallelNodeAttr  = "com.amazon.alexa.behaviors.model.ParallelNode"
	OperationNodeAttr = "com.amazon.alexa.behaviors.model.OpaquePayloadOperationNode"
)

// AlexaInfoSkills is the canonical list of "Info" skill names that
// MediaService.InfoSkill accepts. Each is a SequenceType-shaped wire
// value (Alexa.Calendar.PlayToday, Alexa.Weather.Play, ...). Ported from
// upstream const/metadata.py ALEXA_INFO_SKILLS.
var AlexaInfoSkills = []string{
	"Alexa.Calendar.PlayToday",
	"Alexa.Calendar.PlayTomorrow",
	"Alexa.Calendar.PlayNext",
	"Alexa.Date.Play",
	"Alexa.Time.Play",
	"Alexa.News.NationalNews",
	"Alexa.FlashBriefing.Play",
	"Alexa.Traffic.Play",
	"Alexa.Weather.Play",
	"Alexa.CleanUp.Play",
	"Alexa.GoodMorning.Play",
	"Alexa.SingASong.Play",
	"Alexa.FunFact.Play",
	"Alexa.Joke.Play",
	"Alexa.TellStory.Play",
	"Alexa.ImHome.Play",
	"Alexa.GoodNight.Play",
}

// IsAlexaInfoSkill reports whether name is a recognised Info skill.
func IsAlexaInfoSkill(name string) bool {
	for _, s := range AlexaInfoSkills {
		if s == name {
			return true
		}
	}
	return false
}
