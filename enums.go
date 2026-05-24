package amazondevices

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   SeqSpeak="Alexa.Speak",
//   SeqAnnouncement="AlexaAnnouncement",
//   SeqSound="Alexa.Sound",
//   SeqMusic="Alexa.Music.PlaySearchPhrase",
//   SeqTextCommand="Alexa.TextCommand",
//   SeqLaunchSkill="Alexa.Operation.SkillConnections.Launch",
//   SeqVolume="Alexa.DeviceControls.Volume",
//   SeqStop="Alexa.DeviceControls.Stop",
//   SeqRoutine="Pseudo.Type.Routines"
// )
type SequenceType string

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   CmdPlay="PlayCommand",
//   CmdStop="StopSequence",
//   CmdPause="PauseCommand",
//   CmdNext="NextCommand",
//   CmdPrev="PreviousCommand",
//   CmdRewind="RewindCommand",
//   CmdForward="ForwardCommand"
// )
type MediaCommand string
