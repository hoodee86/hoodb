import React, { useState, useMemo } from 'react';
import {
  ThemeProvider,
  createTheme,
  CssBaseline,
  Container,
  Box,
  AppBar,
  Toolbar,
  Typography,
  Tabs,
  Tab,
  IconButton,
  Tooltip,
  ToggleButtonGroup,
  ToggleButton,
} from '@mui/material';
import StorageIcon from '@mui/icons-material/Storage';
import Brightness4Icon from '@mui/icons-material/Brightness4';
import Brightness7Icon from '@mui/icons-material/Brightness7';
import { useTranslation } from 'react-i18next';
import './i18n';
import KVOperations from './components/KVOperations';
import ClusterStatus from './components/ClusterStatus';
import BatchOperations from './components/BatchOperations';
import PerformanceTest from './components/PerformanceTest';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;
  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`tabpanel-${index}`}
      aria-labelledby={`tab-${index}`}
      {...other}
    >
      {value === index && <Box sx={{ py: 3 }}>{children}</Box>}
    </div>
  );
}

function App() {
  const { t, i18n } = useTranslation();
  const [tabValue, setTabValue] = useState(0);
  const [mode, setMode] = useState<'light' | 'dark'>(
    () => (localStorage.getItem('hoodb-theme') as 'light' | 'dark') || 'light'
  );

  const theme = useMemo(
    () =>
      createTheme({
        palette: {
          mode,
          primary: { main: '#1976d2' },
          secondary: { main: '#dc004e' },
        },
      }),
    [mode]
  );

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
    setTabValue(newValue);
  };

  const toggleTheme = () => {
    setMode((prev) => {
      const next = prev === 'light' ? 'dark' : 'light';
      localStorage.setItem('hoodb-theme', next);
      return next;
    });
  };

  const handleLanguageChange = (_: React.MouseEvent<HTMLElement>, newLang: string | null) => {
    if (newLang) {
      i18n.changeLanguage(newLang);
      localStorage.setItem('hoodb-lang', newLang);
    }
  };

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Box sx={{ flexGrow: 1 }}>
        <AppBar position="static" elevation={1}>
          <Toolbar>
            <StorageIcon sx={{ mr: 2 }} />
            <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
              {t('appTitle')}
            </Typography>
            <Typography variant="body2" sx={{ opacity: 0.8, mr: 2 }}>
              {t('appSubtitle')}
            </Typography>

            <ToggleButtonGroup
              value={i18n.language}
              exclusive
              onChange={handleLanguageChange}
              size="small"
              sx={{
                mr: 1,
                '& .MuiToggleButton-root': {
                  color: 'rgba(255,255,255,0.7)',
                  borderColor: 'rgba(255,255,255,0.3)',
                  py: 0.5,
                  px: 1.5,
                  fontSize: '0.75rem',
                  '&.Mui-selected': {
                    color: '#fff',
                    backgroundColor: 'rgba(255,255,255,0.2)',
                  },
                },
              }}
            >
              <ToggleButton value="en">EN</ToggleButton>
              <ToggleButton value="zh">中文</ToggleButton>
            </ToggleButtonGroup>

            <Tooltip title={mode === 'dark' ? t('lightMode') : t('darkMode')}>
              <IconButton color="inherit" onClick={toggleTheme}>
                {mode === 'dark' ? <Brightness7Icon /> : <Brightness4Icon />}
              </IconButton>
            </Tooltip>
          </Toolbar>
        </AppBar>

        <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <Tabs value={tabValue} onChange={handleTabChange} aria-label="hoodb tabs">
              <Tab label={t('tabKV')} />
              <Tab label={t('tabBatch')} />
              <Tab label={t('tabCluster')} />
              <Tab label={t('tabPerformance')} />
            </Tabs>
          </Box>

          <TabPanel value={tabValue} index={0}>
            <KVOperations />
          </TabPanel>
          <TabPanel value={tabValue} index={1}>
            <BatchOperations />
          </TabPanel>
          <TabPanel value={tabValue} index={2}>
            <ClusterStatus />
          </TabPanel>
          <TabPanel value={tabValue} index={3}>
            <PerformanceTest />
          </TabPanel>
        </Container>
      </Box>
    </ThemeProvider>
  );
}

export default App;
