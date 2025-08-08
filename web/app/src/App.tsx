import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import MainView from './components/MainView';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* Default route redirects to /cells */}
        <Route path="/" element={<Navigate to="/cells" replace />} />
        
        {/* Main cells view */}
        <Route path="/cells" element={<MainView />} />
        
        {/* Cell detail routes */}
        <Route path="/cell/:cellId" element={<MainView />} />
        <Route path="/cell/:cellId/:tab" element={<MainView />} />
        <Route path="/cell/:cellId/:tab/:subtab" element={<MainView />} />
        
        {/* Catch all - redirect to /cells */}
        <Route path="*" element={<Navigate to="/cells" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;