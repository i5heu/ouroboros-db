package messanger

type MessangerService interface {
	Init() error
	SendMessage(message string) error
	SendMessageToPeer(peer string, message string) error
	DistributeMessage(message string, numberOfSuccessors int) error
	GetAllPeers() ([]string, error)
	MessageHandler(handler func(message string) error)
	AddPeer(peer string) error
	Close()
}
