require "ipaddr"

module OpenProject
  module ReplanOverlay
    # Keep the display policy aligned with Go's internal/displayurl package.
    module DisplayUrl
      HTTP_URL = %r{\A(https?)://([^/?\#]+)([/?\#].*)?\z}i
      OPENPROJECT_API_WORK_PACKAGE = %r{\Ahttps?://[^/?\#]+(?:/[^?\#]*)?/api/v3/work_packages(?:[/?\#]|\z)}
      AUTHORITY = /\A(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9.-]+)(?::([0-9]+))?\z/
      DNS_LABEL = /\A[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\z/
      URL_CHARS = %r{\A(?:[A-Za-z0-9:/?\#\[\]@!$&'()*+,;=._~-]|%[0-9A-Fa-f]{2})+\z}
      NUMERIC_HOST_LABEL = /\A(?:[0-9]+|0x[0-9a-f]*)\z/i

      def self.normalize(raw)
        return unless raw.is_a?(String) && raw.ascii_only?

        value = raw.gsub(/\A +| +\z/, "")
        parts = HTTP_URL.match(value)
        return unless parts && URL_CHARS.match?(value)

        host_port = AUTHORITY.match(parts[2])
        return unless host_port && valid_host?(host_port[1])

        if (port = host_port[2])
          digits = port.sub(/\A0+/, "")
          return unless digits.size.between?(1, 5) && digits.to_i <= 65_535
        end

        parts[1].downcase + "://" + parts[2] + parts[3].to_s
      end

      def self.normalize_work_package(raw)
        value = normalize(raw)
        value unless value && OPENPROJECT_API_WORK_PACKAGE.match?(value)
      end

      def self.valid_host?(host)
        return IPAddr.new(host[1...-1]).ipv6? if host.start_with?("[")

        host = host.delete_suffix(".")
        return false if host.empty? || host.size > 253

        labels = host.split(".", -1)
        return IPAddr.new(host).ipv4? if NUMERIC_HOST_LABEL.match?(labels.last)

        labels.all? { |label| DNS_LABEL.match?(label) }
      rescue IPAddr::InvalidAddressError
        false
      end
      private_class_method :valid_host?
    end
  end
end
