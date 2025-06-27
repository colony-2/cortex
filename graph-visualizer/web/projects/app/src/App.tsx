import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import MainView from './components/MainView';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* Default route redirects to /boxes */}
        <Route path="/" element={<Navigate to="/boxes" replace />} />
        
        {/* Main boxes view */}
        <Route path="/boxes" element={<MainView />} />
        
        {/* Box detail routes */}
        <Route path="/box/:boxId" element={<MainView />} />
        <Route path="/box/:boxId/:tab" element={<MainView />} />
        <Route path="/box/:boxId/:tab/:subtab" element={<MainView />} />
        
        {/* Catch all - redirect to /boxes */}
        <Route path="*" element={<Navigate to="/boxes" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;