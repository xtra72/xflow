import { StrictMode } from 'react';
import ReactDOM from 'react-dom/client';

import '@/index.css';
import App from '@/App';
import { apiClient } from '@/services/api/client';
import { setupInterceptors } from '@/services/api/interceptors';

// Attach auth and token-refresh interceptors before the app renders.
setupInterceptors(apiClient);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
