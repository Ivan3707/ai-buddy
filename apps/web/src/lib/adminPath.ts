/** Админка открывается по адресу /admin (пункта в меню нет). */
export const isAdminPath = () => typeof location !== 'undefined' && /^\/admin\/?$/.test(location.pathname);
