import React from 'react';
import { ConfigProvider, Select, theme as antdTheme } from 'antd';
import { useTheme } from '../../contexts/ThemeContext';

const selectTheme = (isDark) => ({
  algorithm: isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
  token: {
    colorPrimary: '#6366f1',
    colorBgContainer: isDark ? '#151f32' : '#ffffff',
    colorBgElevated: isDark ? '#182338' : '#ffffff',
    colorBorder: isDark ? '#334155' : '#d8dde7',
    colorText: isDark ? '#e5e7eb' : '#111827',
    colorTextPlaceholder: isDark ? '#94a3b8' : '#6b7280',
    borderRadius: 7,
    controlHeight: 40,
    controlHeightSM: 32,
    controlHeightLG: 46,
    fontFamily: "'DM Sans', sans-serif",
    fontSize: 13,
  },
  components: {
    Select: {
      optionActiveBg: isDark ? '#26324a' : '#f5f7ff',
      optionSelectedBg: isDark ? '#303b67' : '#eef2ff',
      optionSelectedColor: isDark ? '#c7d2fe' : '#3730a3',
      optionFontSize: 13,
      optionHeight: 34,
      selectorBg: isDark ? '#151f32' : '#ffffff',
    },
  },
});

export default function AppSelect({
  className = '',
  searchable = false,
  width = '100%',
  popupWidth,
  size = 'middle',
  styles,
  ...props
}) {
  const { isDark } = useTheme();
  const searchConfig = searchable ? { optionFilterProp: ['label', 'searchText'] } : false;
  const popupRootStyle = popupWidth ? { minWidth: popupWidth } : undefined;

  return (
    <ConfigProvider theme={selectTheme(isDark)}>
      <Select
        className={`app-select ${className}`.trim()}
        size={size}
        variant="outlined"
        showSearch={searchConfig}
        popupMatchSelectWidth={popupWidth || true}
        styles={{
          ...styles,
          root: { width, ...styles?.root },
          popup: {
            ...styles?.popup,
            root: { ...popupRootStyle, ...styles?.popup?.root },
          },
        }}
        {...props}
      />
    </ConfigProvider>
  );
}
