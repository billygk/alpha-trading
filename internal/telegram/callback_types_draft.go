package telegram

// CallbackHandler defines the signature for processing callback queries.
type CallbackHandler func(callbackID, data string) string

// Update struct extension for CallbackQuery
// Note: We can't easily extend the struct in listener.go without editing it.
// So we will modify listener.go directly.
