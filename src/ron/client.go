
func (r *Ron) startClient() error {
	log.Debugln("startClient")

	if r.serialPath != "" {
		err := r.serialDial()
		if err != nil {
			return err
		}
	}

	// start the periodic query to the parent
	go r.heartbeat()

	return nil
}
