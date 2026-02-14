import React, { useState } from 'react';
import {
  Card,
  CardContent,
  TextField,
  Button,
  Box,
  Typography,
  Alert,
  Grid,
  Paper,
  IconButton,
  Tooltip,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import GetAppIcon from '@mui/icons-material/GetApp';
import DeleteIcon from '@mui/icons-material/Delete';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useTranslation } from 'react-i18next';
import { getKey, setKey, deleteKey } from '../services/api';

const KVOperations: React.FC = () => {
  const { t } = useTranslation();
  const [key, setKeyInput] = useState('');
  const [value, setValue] = useState('');
  const [result, setResult] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleGet = async () => {
    if (!key) {
      setError(t('kvPleaseEnterKey'));
      return;
    }
    setLoading(true);
    setError('');
    setResult('');
    try {
      const val = await getKey(key);
      setResult(val);
      setValue(val);
    } catch (err: any) {
      setError(err.response?.data?.error || err.message || t('kvGetFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleSet = async () => {
    if (!key || !value) {
      setError(t('kvPleaseEnterBoth'));
      return;
    }
    setLoading(true);
    setError('');
    setResult('');
    try {
      await setKey(key, value);
      setResult(t('kvSetSuccess', { key }));
    } catch (err: any) {
      setError(err.response?.data?.error || err.message || t('kvSetFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!key) {
      setError(t('kvPleaseEnterKey'));
      return;
    }
    setLoading(true);
    setError('');
    setResult('');
    try {
      await deleteKey(key);
      setResult(t('kvDeleteSuccess', { key }));
      setValue('');
    } catch (err: any) {
      setError(err.response?.data?.error || err.message || t('kvDeleteFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(result);
  };

  return (
    <Box>
      <Typography variant="h5" gutterBottom>
        {t('kvTitle')}
      </Typography>
      <Typography variant="body2" color="text.secondary" paragraph>
        {t('kvDescription')}
      </Typography>

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 6 }}>
          <Card elevation={2}>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t('kvInput')}
              </Typography>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <TextField
                  label={t('kvKey')}
                  value={key}
                  onChange={(e) => setKeyInput(e.target.value)}
                  placeholder={t('kvKeyPlaceholder')}
                  fullWidth
                  variant="outlined"
                />
                <TextField
                  label={t('kvValue')}
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                  placeholder={t('kvValuePlaceholder')}
                  fullWidth
                  multiline
                  rows={4}
                  variant="outlined"
                />
                <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                  <Button
                    variant="contained"
                    startIcon={<AddIcon />}
                    onClick={handleSet}
                    disabled={loading}
                  >
                    {t('kvSet')}
                  </Button>
                  <Button
                    variant="outlined"
                    startIcon={<GetAppIcon />}
                    onClick={handleGet}
                    disabled={loading}
                  >
                    {t('kvGet')}
                  </Button>
                  <Button
                    variant="outlined"
                    color="error"
                    startIcon={<DeleteIcon />}
                    onClick={handleDelete}
                    disabled={loading}
                  >
                    {t('kvDelete')}
                  </Button>
                </Box>
              </Box>
            </CardContent>
          </Card>
        </Grid>

        <Grid size={{ xs: 12, md: 6 }}>
          <Card elevation={2}>
            <CardContent>
              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                <Typography variant="h6">
                  {t('kvResult')}
                </Typography>
                {result && (
                  <Tooltip title={t('kvCopyToClipboard')}>
                    <IconButton size="small" onClick={handleCopy}>
                      <ContentCopyIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                )}
              </Box>

              {error && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {error}
                </Alert>
              )}

              {result && (
                <Alert severity="success" sx={{ mb: 2 }}>
                  {result}
                </Alert>
              )}

              {!error && !result && (
                <Paper
                  variant="outlined"
                  sx={{
                    p: 2,
                    minHeight: 150,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    bgcolor: 'grey.50',
                  }}
                >
                  <Typography color="text.secondary">
                    {t('kvResultPlaceholder')}
                  </Typography>
                </Paper>
              )}
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      <Card elevation={2} sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            {t('kvQuickExamples')}
          </Typography>
          <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
            <Button
              size="small"
              variant="outlined"
              onClick={() => {
                setKeyInput('test:1');
                setValue('Hello HooDB!');
              }}
            >
              {t('kvSimpleString')}
            </Button>
            <Button
              size="small"
              variant="outlined"
              onClick={() => {
                setKeyInput('user:1001');
                setValue('{"name":"Alice","email":"alice@example.com","age":30}');
              }}
            >
              {t('kvJsonObject')}
            </Button>
            <Button
              size="small"
              variant="outlined"
              onClick={() => {
                setKeyInput('session:abc123');
                setValue('{"userId":1001,"token":"xyz789","expires":"2026-12-31"}');
              }}
            >
              {t('kvSessionData')}
            </Button>
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
};

export default KVOperations;
