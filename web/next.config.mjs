/** @type {import('next').NextConfig} */
const standalone = process.env.NEXT_OUTPUT === 'standalone';

const nextConfig = {
  output: standalone ? 'standalone' : 'export',
  images: { unoptimized: true },
};

export default nextConfig;
