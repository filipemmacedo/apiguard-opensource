type BrandLogoProps = {
  className?: string
  imageClassName?: string
  lightSurface?: boolean
}

export function BrandLogo({
  className = '',
  imageClassName = 'h-8 w-auto',
  lightSurface = false,
}: BrandLogoProps) {
  return (
    <div className={`inline-flex items-center ${className}`.trim()}>
      <img
        src="/assets/logos/Logo_APIguard_WHITE.svg"
        alt="APIguard"
        className={`block ${imageClassName}`.trim()}
        decoding="async"
      />
    </div>
  )
}
