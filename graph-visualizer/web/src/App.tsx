import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import GraphFlow from './components/GraphFlow';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* Default route redirects to /boxes */}
        <Route path="/" element={<Navigate to="/boxes" replace />} />
        
        {/* Main boxes view */}
        <Route path="/boxes" element={<GraphFlow />} />
        
        {/* Box detail routes */}
        <Route path="/box/:boxId" element={<GraphFlow />} />
        <Route path="/box/:boxId/:tab" element={<GraphFlow />} />
        <Route path="/box/:boxId/:tab/:subtab" element={<GraphFlow />} />
        
        {/* Catch all - redirect to /boxes */}
        <Route path="*" element={<Navigate to="/boxes" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;