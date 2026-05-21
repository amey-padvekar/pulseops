import { useApiBaseUrl } from './hooks/useApiBaseUrl'
import { DashboardPage } from './pages/DashboardPage'
import './App.css'

function App() {
  const apiBaseUrl = useApiBaseUrl()

  return <DashboardPage apiBaseUrl={apiBaseUrl} />
}

export default App
