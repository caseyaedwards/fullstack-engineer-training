// region: app
import { newClient } from './api'
import { StatusPage } from './StatusPage'

// One client for the life of the page. Made inside App, it is a new
// object on every render, and every useCallback that lists it restarts.
const client = newClient()

function App() {
  return <StatusPage client={client} />
}

export default App
// endregion: app
