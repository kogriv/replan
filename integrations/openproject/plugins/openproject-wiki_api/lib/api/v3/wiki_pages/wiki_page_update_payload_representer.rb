# frozen_string_literal: true

module API
  module V3
    module WikiPages
      class WikiPageUpdatePayloadRepresenter < WikiPagePayloadRepresenter
        def initialize(represented, current_user:, update: true, embed_links: false)
          super(represented, current_user:, update: true, embed_links:)
        end
      end
    end
  end
end
