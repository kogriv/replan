# frozen_string_literal: true

module API
  module V3
    module WikiPages
      class WikiPageCollectionRepresenter < ::API::Decorators::OffsetPaginatedCollection
        element_decorator ::API::V3::WikiPages::WikiPageListItemRepresenter
      end
    end
  end
end
