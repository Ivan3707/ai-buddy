import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import '@fontsource-variable/manrope';
import './styles/theme.css';
import './styles/auth.css';
import './styles/shell.css';
import './styles/home.css';
import './styles/feedback.css';
import './legacy.css';

createRoot(document.getElementById('root')!).render(<React.StrictMode><App /></React.StrictMode>);
