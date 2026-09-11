// Modified by Variya, 2026-09-11: independent workspace document titles.
import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { brand } from '../brand';

const titles: Record<string, string> = { '/': '工作台', '/dashboard': '工作台', '/accounts': '账号管理', '/settings': '设置' };
export default function useDocumentTitle() {
  const location = useLocation();
  useEffect(() => {
    const page = location.pathname.startsWith('/accounts/') ? '账号详情' : titles[location.pathname] || '工作台';
    document.title = `${page} | ${brand.name} by ${brand.maintainer}`;
  }, [location.pathname]);
}
