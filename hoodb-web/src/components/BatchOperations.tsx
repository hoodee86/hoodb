import React, { useState } from 'react';
import {
  Card,
  CardContent,
  TextField,
  Button,
  Box,
  Typography,
  Alert,
  IconButton,
  Paper,
  Chip,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import SendIcon from '@mui/icons-material/Send';
import { useTranslation } from 'react-i18next';
import { batchSet } from '../services/api';

interface KeyValuePair {
  id: number;
  key: string;
  value: string;
}

const BatchOperations: React.FC = () => {
  const { t } = useTranslation();
  const [pairs, setPairs] = useState<KeyValuePair[]>([
    { id: 1, key: '', value: '' },
  ]);
  const [result, setResult] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [nextId, setNextId] = useState(2);

  const handleAddPair = () => {
    setPairs([...pairs, { id: nextId, key: '', value: '' }]);
    setNextId(nextId + 1);
  };

  const handleRemovePair = (id: number) => {
    if (pairs.length > 1) {
      setPairs(pairs.filter((p) => p.id !== id));
    }
  };

  const handlePairChange = (id: number, field: 'key' | 'value', value: string) => {
    setPairs(
      pairs.map((p) => (p.id === id ? { ...p, [field]: value } : p))
    );
  };

  const handleBatchSet = async () => {
    const validPairs = pairs.filter((p) => p.key && p.value);
    if (validPairs.length === 0) {
      setError(t('batchNeedOne'));
      return;
    }

    setLoading(true);
    setError('');
    setResult('');

    try {
      const items: { [key: string]: string } = {};
      validPairs.forEach((p) => {
        items[p.key] = p.value;
      });

      await batchSet(items);
      setResult(t('batchSuccess', { count: validPairs.length }));
    } catch (err: any) {
      setError(err.response?.data?.error || err.message || t('batchFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleLoadExample = () => {
    setPairs([
      { id: 1, key: 'user:1001', value: '{"name":"Alice","role":"admin"}' },
      { id: 2, key: 'user:1002', value: '{"name":"Bob","role":"user"}' },
      { id: 3, key: 'user:1003', value: '{"name":"Charlie","role":"user"}' },
      { id: 4, key: 'config:db', value: '{"host":"localhost","port":5432}' },
      { id: 5, key: 'config:cache', value: '{"ttl":3600,"maxSize":1000}' },
    ]);
    setNextId(6);
  };

  const handleGenerateBatch = () => {
    const newPairs: KeyValuePair[] = [];
    for (let i = 0; i < 50; i++) {
      newPairs.push({
        id: nextId + i,
        key: `batch_${Date.now()}_${i}`,
        value: `value_${i}_${Math.random().toString(36).substr(2, 9)}`,
      });
    }
    setPairs(newPairs);
    setNextId(nextId + 50);
  };

  return (
    <Box>
      <Typography variant="h5" gutterBottom>
        {t('batchTitle')}
      </Typography>
      <Typography variant="body2" color="text.secondary" paragraph>
        {t('batchDescription')}
      </Typography>

      <Card elevation={2} sx={{ mb: 3 }}>
        <CardContent>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <Typography variant="h6">
              {t('batchKeyValuePairs')}
              <Chip label={t('batchPairs', { count: pairs.length })} size="small" sx={{ ml: 1 }} />
            </Typography>
            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button size="small" onClick={handleLoadExample}>
                {t('batchLoadExample')}
              </Button>
              <Button size="small" onClick={handleGenerateBatch}>
                {t('batchGenerate50')}
              </Button>
              <Button
                variant="contained"
                startIcon={<AddIcon />}
                onClick={handleAddPair}
                size="small"
              >
                {t('batchAddPair')}
              </Button>
            </Box>
          </Box>

          <Paper
            variant="outlined"
            sx={{
              p: 2,
              maxHeight: 400,
              overflow: 'auto',
              bgcolor: 'action.hover',
            }}
          >
            {pairs.map((pair, index) => (
              <Box
                key={pair.id}
                sx={{
                  display: 'flex',
                  gap: 1,
                  mb: 1,
                  alignItems: 'center',
                }}
              >
                <Chip label={index + 1} size="small" sx={{ minWidth: 40 }} />
                <TextField
                  size="small"
                  placeholder={t('kvKey')}
                  value={pair.key}
                  onChange={(e) => handlePairChange(pair.id, 'key', e.target.value)}
                  sx={{ flex: 1 }}
                />
                <TextField
                  size="small"
                  placeholder={t('kvValue')}
                  value={pair.value}
                  onChange={(e) => handlePairChange(pair.id, 'value', e.target.value)}
                  sx={{ flex: 2 }}
                />
                <IconButton
                  size="small"
                  color="error"
                  onClick={() => handleRemovePair(pair.id)}
                  disabled={pairs.length === 1}
                >
                  <DeleteIcon />
                </IconButton>
              </Box>
            ))}
          </Paper>

          <Box sx={{ display: 'flex', justifyContent: 'center', mt: 2 }}>
            <Button
              variant="contained"
              size="large"
              startIcon={<SendIcon />}
              onClick={handleBatchSet}
              disabled={loading}
            >
              {t('batchSet', { count: pairs.filter((p) => p.key && p.value).length })}
            </Button>
          </Box>
        </CardContent>
      </Card>

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

      <Card elevation={2}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            {t('batchPerformanceTips')}
          </Typography>
          <Typography variant="body2" color="text.secondary" paragraph>
            {t('batchTip1')}
          </Typography>
          <Typography variant="body2" color="text.secondary" paragraph dangerouslySetInnerHTML={{ __html: t('batchTip2') }} />
          <Typography variant="body2" color="text.secondary">
            {t('batchTip3')}
          </Typography>
        </CardContent>
      </Card>
    </Box>
  );
};

export default BatchOperations;
